package localpostgres

import (
	"bytes"
	"os"
	"os/exec"
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
