package localpostgres

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

// PostgreSQL refuses an elevated administrator or Power Users token. Like
// initdb/pg_ctl, run it with those groups disabled and excess privileges removed,
// while keeping the direct child process and ARTEX's job-object containment.
func startPostgres(cmd *exec.Cmd) error {
	token, err := postgresToken()
	if err != nil {
		return err
	}
	defer token.Close()
	cmd.SysProcAttr.Token = syscall.Token(token)
	return cmd.Start()
}

func postgresToken() (windows.Token, error) {
	var original windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ALL_ACCESS, &original); err != nil {
		return 0, fmt.Errorf("open database process token: %w", err)
	}
	defer original.Close()
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return 0, err
	}
	powerUsers, err := windows.CreateWellKnownSid(windows.WinBuiltinPowerUsersSid)
	if err != nil {
		return 0, err
	}
	drop := []windows.SIDAndAttributes{{Sid: admins}, {Sid: powerUsers}}
	var token windows.Token
	const disableMaxPrivilege = 0x1
	createRestrictedToken := windows.NewLazySystemDLL("advapi32.dll").NewProc("CreateRestrictedToken")
	ok, _, callErr := createRestrictedToken.Call(uintptr(original), disableMaxPrivilege,
		uintptr(len(drop)), uintptr(unsafe.Pointer(&drop[0])), 0, 0, 0, 0, uintptr(unsafe.Pointer(&token)))
	runtime.KeepAlive(drop)
	if ok == 0 {
		return 0, fmt.Errorf("restrict database process token: %w", callErr)
	}
	if err := allowTokenUser(token); err != nil {
		token.Close()
		return 0, fmt.Errorf("set database token default ACL: %w", err)
	}
	return token, nil
}

// Elevated tokens may grant access to new processes/pipes only to Administrators
// and SYSTEM. Preserve the existing default ACL and add the actual user before
// disabling the administrator groups, as PostgreSQL's AddUserToTokenDacl does.
func allowTokenUser(token windows.Token) error {
	var size uint32
	err := windows.GetTokenInformation(token, windows.TokenDefaultDacl, nil, 0, &size)
	if err != windows.ERROR_INSUFFICIENT_BUFFER {
		return err
	}
	buffer := make([]byte, size)
	if err := windows.GetTokenInformation(token, windows.TokenDefaultDacl, &buffer[0], size, &size); err != nil {
		return err
	}
	type defaultDACL struct{ ACL *windows.ACL }
	current := (*defaultDACL)(unsafe.Pointer(&buffer[0]))
	if current.ACL == nil { // An unrestricted default ACL already permits the user.
		return nil
	}
	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	var pin runtime.Pinner
	pin.Pin(user.User.Sid)
	defer pin.Unpin()
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       windows.OBJECT_INHERIT_ACE,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid),
		},
	}}, current.ACL)
	runtime.KeepAlive(buffer)
	if err != nil {
		return err
	}
	updated := defaultDACL{ACL: acl}
	err = windows.SetTokenInformation(token, windows.TokenDefaultDacl,
		(*byte)(unsafe.Pointer(&updated)), uint32(unsafe.Sizeof(updated)))
	runtime.KeepAlive(acl)
	return err
}

// A non-inheritable job handle binds the database and all its workers to ARTEX.
// Windows closes it even after a crash or console close, preventing orphan servers.
func containProcess(process *os.Process) (func(), error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(process.Pid))
	if err == nil {
		err = windows.AssignProcessToJobObject(job, handle)
		windows.CloseHandle(handle)
	}
	if err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	return func() { windows.CloseHandle(job) }, nil
}

func lockFile(file *os.File) error {
	return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
}
