@echo off
rem Keep this entire script ASCII so cmd.exe can parse it in any code page.
chcp 65001 >nul 2>&1
rem ARTEX Windows launcher.
rem
rem Usage: start.bat [-addr :9000] [other artex options]
rem Press Ctrl-C to stop. All arguments are forwarded to artex.exe.
rem
rem Exit 0: stop; exit 75: restart after update/rollback.
rem Other exit codes: retry with exponential backoff, up to 60 seconds.
rem Downloads, verification and binary replacement are handled by artex itself.

setlocal enabledelayedexpansion
cd /d "%~dp0"

set "BIN=artex.exe"
if not exist "%BIN%" (
	echo [artex] Executable not found: %BIN% 1>&2
	exit /b 1
)

set "RESTART_CODE=75"
set "MAX_DELAY=60"
set /a delay=1

:loop
"%BIN%" %*
set "code=!ERRORLEVEL!"

if "!code!"=="0" (
	echo [artex] Stopped normally.
	exit /b 0
)

if "!code!"=="%RESTART_CODE%" (
	rem The next process applies the staged update or rollback at startup.
	echo [artex] Restart requested. Applying staged version.
	set /a delay=1
	goto loop
)

echo [artex] Unexpected exit ^(code=!code!^). Restarting in !delay!s. 1>&2
rem Unlike timeout, ping also works when the console input is redirected.
set /a pings=!delay!+1
ping -n !pings! 127.0.0.1 >nul 2>&1
set /a delay=!delay!*2
if !delay! gtr %MAX_DELAY% set /a delay=%MAX_DELAY%
goto loop
