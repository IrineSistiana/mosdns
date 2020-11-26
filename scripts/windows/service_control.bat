@echo off

::    Copyright (C) 2020, IrineSistiana
::
::    This file is part of mosdns.
::
::    mosdns is free software: you can redistribute it and/or modify
::    it under the terms of the GNU General Public License as published by
::    the Free Software Foundation, either version 3 of the License, or
::    (at your option) any later version.
::
::    mosdns is distributed in the hope that it will be useful,
::    but WITHOUT ANY WARRANTY; without even the implied warranty of
::    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
::    GNU General Public License for more details.
::
::    You should have received a copy of the GNU General Public License
::    along with this program.  If not, see <https://www.gnu.org/licenses/>.

set WINSW_BIN=mosdns-winsw.exe
set MOSDNS_BIN=mosdns.exe

:: Administrative check
net session >nul 2>&1
if %errorLevel% == 0 (
    echo Administrative access granted
) else (
    echo Please run as Administrator
	pause
	exit
)

cd /d "%~dp0"

if not exist %WINSW_BIN% (
    echo Error: winsw not find
	pause
	exit
) 

:select
echo ========================
echo 1: Install service
echo 2: Uninstall service
echo 3: Start service
echo 4: Stop service
echo 5: Restart service
echo 6: Flush system DNS cache
echo 7: Check status
echo 9: Exit
echo =========================
set /P case="Enter the number and press enter: "
set case=case_%case%
cls
goto %case%

:: install
:case_1
	%WINSW_BIN% stop
	%WINSW_BIN% uninstall
	%WINSW_BIN% install
	%WINSW_BIN% start
	echo.
	pause
	goto :select


:: uninstall
:case_2
	%WINSW_BIN% stop
	%WINSW_BIN% uninstall
	echo.
	pause
	goto :select

:: start
:case_3
	%WINSW_BIN% start
	echo.
	pause
	goto :select

:: stop
:case_4
	%WINSW_BIN% stop
	echo.
	pause
	goto :select

:: restart
:case_5
	%WINSW_BIN% restart
	echo.
	pause
	goto :select

:: Flush DNS cache
:case_6
	ipconfig /flushdns
	echo.
	pause
	goto :select


:: Status check
:case_7
	tasklist /FI "IMAGENAME eq %WINSW_BIN%" 2>NUL | find /I /N "%WINSW_BIN%">NUL
	if "%ERRORLEVEL%"=="0" (
		echo winsw %WINSW_BIN% is running as service.
	) else (
		echo Error: %WINSW_BIN% is not running. error log might have more information.
	)

	tasklist /FI "IMAGENAME eq %MOSDNS_BIN%" 2>NUL | find /I /N "%MOSDNS_BIN%">NUL
	if "%ERRORLEVEL%"=="0" (
		echo mosdns %MOSDNS_BIN% is running in the background.
	) else (
		echo Error: %MOSDNS_BIN% is not running. error log might have more information.
	)
	echo.
	pause
	goto :select

:case_9
	echo bye
	exit
