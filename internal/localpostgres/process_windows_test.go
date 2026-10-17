package localpostgres

import (
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestPostgresChildToken(t *testing.T) {
	if os.Getenv("ARTEX_TEST_RESTRICTED_TOKEN") == "1" {
		token := windows.GetCurrentProcessToken()
		groups, err := token.GetTokenGroups()
		if err != nil {
			t.Fatal(err)
		}
		for _, kind := range []windows.WELL_KNOWN_SID_TYPE{windows.WinBuiltinAdministratorsSid, windows.WinBuiltinPowerUsersSid} {
			sid, err := windows.CreateWellKnownSid(kind)
			if err != nil {
				t.Fatal(err)
			}
			for _, group := range groups.AllGroups() {
				if group.Sid.Equals(sid) && group.Attributes&windows.SE_GROUP_ENABLED != 0 {
					t.Fatalf("database child retains administrative group %s", sid)
				}
			}
		}
		var notify windows.LUID
		if err := windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr("SeChangeNotifyPrivilege"), &notify); err != nil {
			t.Fatal(err)
		}
		var size uint32
		if err := windows.GetTokenInformation(token, windows.TokenPrivileges, nil, 0, &size); err != windows.ERROR_INSUFFICIENT_BUFFER {
			t.Fatal(err)
		}
		buffer := make([]byte, size)
		if err := windows.GetTokenInformation(token, windows.TokenPrivileges, &buffer[0], size, &size); err != nil {
			t.Fatal(err)
		}
		privileges := (*windows.Tokenprivileges)(unsafe.Pointer(&buffer[0]))
		for _, privilege := range privileges.AllPrivileges() {
			if privilege.Luid != notify {
				t.Fatalf("database child retains an unnecessary privilege: %+v", privilege)
			}
		}
		return
	}
	t.Logf("parent token elevated: %t", windows.GetCurrentProcessToken().IsElevated())
	cmd := exec.Command(os.Args[0], "-test.run=^TestPostgresChildToken$", "-test.v")
	cmd.Env = append(os.Environ(), "ARTEX_TEST_RESTRICTED_TOKEN=1")
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	configureCommand(cmd)
	if err := startPostgres(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("restricted database child failed: %v\n%s", err, output.String())
	}
}

func TestRestrictedTokenCanReadPrivateTempFiles(t *testing.T) {
	token, err := postgresToken()
	if err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	var impersonation windows.Token
	if err := windows.DuplicateTokenEx(token, windows.TOKEN_QUERY, nil,
		windows.SecurityImpersonation, windows.TokenImpersonation, &impersonation); err != nil {
		t.Fatal(err)
	}
	defer impersonation.Close()
	// Model the actual Python temp ACL when its elevated creator's default
	// owner is Administrators. AccessCheck models this even on a normal desktop.
	acl := "O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FA;;;OW)"
	for _, tt := range []struct {
		name, sddl string
		allowed    bool
	}{
		{"owner rights only", acl, false},
		{"explicit token user", acl + "(A;OICI;FA;;;" + user.User.Sid.String() + ")", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sd, err := windows.SecurityDescriptorFromString(tt.sddl)
			if err != nil {
				t.Fatal(err)
			}
			mapping := struct{ read, write, execute, all uint32 }{
				windows.FILE_GENERIC_READ, windows.FILE_GENERIC_WRITE,
				windows.FILE_GENERIC_EXECUTE, windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1ff,
			}
			var privileges [1024]byte
			size := uint32(len(privileges))
			var granted, allowed uint32
			check := windows.NewLazySystemDLL("advapi32.dll").NewProc("AccessCheck")
			ok, _, callErr := check.Call(uintptr(unsafe.Pointer(sd)), uintptr(impersonation),
				windows.FILE_GENERIC_READ|windows.FILE_GENERIC_EXECUTE, uintptr(unsafe.Pointer(&mapping)),
				uintptr(unsafe.Pointer(&privileges[0])), uintptr(unsafe.Pointer(&size)),
				uintptr(unsafe.Pointer(&granted)), uintptr(unsafe.Pointer(&allowed)))
			runtime.KeepAlive(sd)
			if ok == 0 {
				t.Fatal(callErr)
			}
			if got := allowed != 0; got != tt.allowed {
				t.Fatalf("restricted database read/execute access = %v, want %v", got, tt.allowed)
			}
		})
	}
}
