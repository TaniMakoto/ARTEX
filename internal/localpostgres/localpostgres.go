// Package localpostgres manages the private PostgreSQL runtime shipped in the
// Windows portable package. It never downloads software or installs a service.
package localpostgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Server struct {
	dsn        string
	data       string
	binaries   string
	cmd        *exec.Cmd
	exited     chan struct{}
	waitErr    error
	log        *os.File
	lock       *os.File
	releaseJob func()
	once       sync.Once
	closeErr   error
}

func executable(dir, name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, "bin", name)
}

// Start keeps persistent state under dataDir/postgres, with a random password
// and a dynamically allocated loopback port. The immutable runtime is separate.
func Start(dataDir, binaries string) (_ *Server, err error) {
	binaries, err = filepath.Abs(binaries)
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"postgres", "initdb", "pg_ctl"} {
		if _, err := os.Stat(executable(binaries, name)); err != nil {
			return nil, fmt.Errorf("缺少免安装数据库组件 %s；请完整解压 Windows portable 包（保留 postgres 目录）: %w", name, err)
		}
	}
	root, err := filepath.Abs(filepath.Join(dataDir, "postgres"))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	s := &Server{data: filepath.Join(root, "cluster"), binaries: binaries}
	s.lock, err = os.OpenFile(filepath.Join(root, "runtime.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(s.lock); err != nil {
		s.lock.Close()
		return nil, fmt.Errorf("此 data 目录正被另一个 ARTEX 实例使用: %w", err)
	}
	defer func() {
		if err != nil {
			_ = s.Close()
		}
	}()
	password, err := persistentPassword(root)
	if err != nil {
		return nil, err
	}
	if err := initialize(root, s.data, binaries, password); err != nil {
		return nil, err
	}
	s.log, err = os.OpenFile(filepath.Join(root, "server.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	// Release the reservation immediately before starting postgres. If another
	// process wins the port, startup fails visibly instead of attaching to it.
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	u := url.URL{Scheme: "postgres", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), Path: "/postgres", User: url.UserPassword("artex", password)}
	u.RawQuery = "sslmode=disable&connect_timeout=2"
	s.cmd = exec.Command(executable(binaries, "postgres"), "-D", s.data, "-h", "127.0.0.1", "-p", strconv.Itoa(port), "-c", "unix_socket_directories=", "-c", "max_connections=100")
	configureCommand(s.cmd)
	s.cmd.Stdout, s.cmd.Stderr = s.log, s.log
	if err := s.cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动随包数据库失败: %w", err)
	}
	s.exited = make(chan struct{})
	go func() {
		s.waitErr = s.cmd.Wait()
		close(s.exited)
	}()
	s.releaseJob, err = containProcess(s.cmd.Process)
	if err != nil {
		return nil, fmt.Errorf("保护数据库子进程失败: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	conn, err := sql.Open("pgx", u.String())
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	for {
		select {
		case <-s.exited:
			return nil, fmt.Errorf("随包数据库提前退出（详见 %s）: %v", s.log.Name(), s.waitErr)
		default:
		}
		if err = conn.PingContext(ctx); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("随包数据库启动超时（详见 %s）: %w", s.log.Name(), err)
		case <-s.exited:
			return nil, fmt.Errorf("随包数据库提前退出（详见 %s）: %v", s.log.Name(), s.waitErr)
		case <-time.After(100 * time.Millisecond):
		}
	}
	var exists bool
	if err := conn.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname='artex')").Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		if _, err := conn.ExecContext(ctx, "CREATE DATABASE artex"); err != nil {
			return nil, err
		}
	}
	u.Path = "/artex"
	s.dsn = u.String()
	return s, nil
}

func persistentPassword(root string) (string, error) {
	path := filepath.Join(root, "password")
	if b, err := os.ReadFile(path); err == nil {
		password := strings.TrimSpace(string(b))
		if len(password) != 64 {
			return "", fmt.Errorf("数据库密码文件格式无效: %s", path)
		}
		if _, err := hex.DecodeString(password); err != nil {
			return "", fmt.Errorf("数据库密码文件格式无效: %s", path)
		}
		return password, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(root, "cluster")); !os.IsNotExist(err) {
		return "", fmt.Errorf("已有数据库缺少密码文件；请恢复完整 data/postgres 备份")
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	password := hex.EncodeToString(secret)
	return password, os.WriteFile(path, []byte(password+"\n"), 0600)
}

func initialize(root, data, binaries, password string) error {
	if _, err := os.Stat(filepath.Join(data, "PG_VERSION")); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(data); !os.IsNotExist(err) {
		return fmt.Errorf("数据库目录不完整，已保留原文件，请检查 %s", data)
	}
	staging, err := os.MkdirTemp(root, "initialize-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	passFile, err := os.CreateTemp(root, "init-password-")
	if err != nil {
		return err
	}
	defer os.Remove(passFile.Name())
	_, writeErr := passFile.WriteString(password + "\n")
	if err := errors.Join(writeErr, passFile.Close()); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable(binaries, "initdb"), "-D", staging, "-U", "artex", "--auth=scram-sha-256", "--pwfile="+passFile.Name(), "--encoding=UTF8", "--locale=C", "--no-instructions")
	configureCommand(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("初始化随包数据库失败: %w\n%s", err, out)
	}
	return os.Rename(staging, data)
}

func (s *Server) DSN() string { return s.dsn }

func (s *Server) Close() error {
	s.once.Do(func() {
		if s.exited != nil {
			select {
			case <-s.exited:
			default:
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				cmd := exec.CommandContext(ctx, executable(s.binaries, "pg_ctl"), "stop", "-D", s.data, "-m", "fast", "-w", "-t", "15")
				configureCommand(cmd)
				out, err := cmd.CombinedOutput()
				cancel()
				if err != nil {
					s.closeErr = fmt.Errorf("停止随包数据库失败: %w\n%s", err, out)
					_ = s.cmd.Process.Kill()
				}
				if s.releaseJob != nil {
					s.releaseJob()
					s.releaseJob = nil
				}
				<-s.exited
			}
		}
		if s.releaseJob != nil {
			s.releaseJob()
		}
		if s.log != nil {
			_ = s.log.Close()
		}
		if s.lock != nil {
			_ = s.lock.Close()
		}
	})
	return s.closeErr
}
