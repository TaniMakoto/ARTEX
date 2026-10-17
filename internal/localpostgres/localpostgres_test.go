package localpostgres

import (
	"bufio"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func testBinaries(t *testing.T) string {
	t.Helper()
	path := os.Getenv("ARTEX_TEST_POSTGRES_DIR")
	if path == "" {
		t.Skip("set ARTEX_TEST_POSTGRES_DIR to the unpacked portable runtime")
	}
	return path
}

func TestPersistenceLockAndAuthentication(t *testing.T) {
	binaries, data := testBinaries(t), filepath.Join(t.TempDir(), "中文 data with spaces")
	s, err := Start(data, binaries)
	if err != nil {
		t.Fatal(err)
	}
	first := s
	t.Cleanup(func() { _ = first.Close() })
	conn, err := sql.Open("pgx", s.DSN())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec("CREATE TABLE portable_test (value text); INSERT INTO portable_test VALUES ('持久数据')"); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(s.DSN())
	u.User = url.UserPassword("artex", "wrong-password")
	wrong, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := wrong.Ping(); err == nil {
		t.Fatal("loopback database must require its random password")
	}
	wrong.Close()
	if other, err := Start(data, binaries); err == nil {
		other.Close()
		t.Fatal("two instances must not share a data directory")
	}
	conn.Close()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Start(data, binaries)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	conn, err = sql.Open("pgx", s.DSN())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var value string
	if err := conn.QueryRow("SELECT value FROM portable_test").Scan(&value); err != nil || value != "持久数据" {
		t.Fatalf("persistent data lost: value=%q err=%v", value, err)
	}
}

func TestMissingRuntime(t *testing.T) {
	if _, err := Start(t.TempDir(), t.TempDir()); err == nil || !strings.Contains(err.Error(), "完整解压") {
		t.Fatalf("missing runtime should produce actionable error: %v", err)
	}
}

func TestCrashCleanup(t *testing.T) {
	if os.Getenv("ARTEX_POSTGRES_TEST_CHILD") == "1" {
		s, err := Start(os.Getenv("ARTEX_POSTGRES_TEST_DATA"), os.Getenv("ARTEX_TEST_POSTGRES_DIR"))
		if err != nil {
			fmt.Println("ERROR", err)
			os.Exit(1)
		}
		u, _ := url.Parse(s.DSN())
		fmt.Println("READY", u.Host)
		select {}
	}
	if runtime.GOOS != "windows" {
		t.Skip("portable package uses Windows job-object containment")
	}
	_ = testBinaries(t)
	data := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashCleanup$")
	cmd.Env = append(os.Environ(), "ARTEX_POSTGRES_TEST_CHILD=1", "ARTEX_POSTGRES_TEST_DATA="+data)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	line := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			line <- scanner.Text()
		} else {
			line <- ""
		}
	}()
	var address string
	select {
	case text := <-line:
		if !strings.HasPrefix(text, "READY ") {
			t.Fatalf("child failed to start: %s", text)
		}
		address = strings.TrimPrefix(text, "READY ")
	case <-time.After(120 * time.Second):
		t.Fatal("child startup timed out")
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	deadline := time.Now().Add(15 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", address, time.Second)
		if err != nil {
			break
		}
		conn.Close()
		if time.Now().After(deadline) {
			t.Fatal("database still listening after its owner was killed")
		}
		time.Sleep(100 * time.Millisecond)
	}
	// An unclean exit must still allow recovery with the same persistent data.
	s, err := Start(data, testBinaries(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
