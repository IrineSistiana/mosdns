@echo off

::    Copyright (C) 2020-2021, IrineSistiana
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

set MOSDNS_BIN=mosdns.exe
set MOSDNS_CONF=config.yaml

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

if not exist %MOSDNS_BIN% (
    echo Error: mosdns.exe not find
	pause
	exit
)

if not exist %MOSDNS_CONF% (
    echo Error: mosdns config file config.yaml not find
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
	%MOSDNS_BIN% -s stop
	%MOSDNS_BIN% -s uninstall
	%MOSDNS_BIN% -s install
	%MOSDNS_BIN% -s start
	echo.
	pause
	goto :select

:: uninstall
:case_2
	%MOSDNS_BIN% -s stop
	%MOSDNS_BIN% -s uninstall
	echo.
	pause
	goto :select

:: start
:case_3
	%MOSDNS_BIN% -s start
	echo.
	pause
	goto :select

:: stop
:case_4
	%MOSDNS_BIN% -s stop
	echo.
	pause
	goto :select

:: restart
:case_5
	%MOSDNS_BIN% -s restart
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
	tasklist /FI "IMAGENAME eq %MOSDNS_BIN%" 2>NUL | find /I /N "%MOSDNS_BIN%">NUL
	if "%ERRORLEVEL%"=="0" (
		echo %MOSDNS_BIN% is running in the background.
	) else (
		echo Error: %MOSDNS_BIN% is not running. error log might have more information.
	)
	echo.
	pause
	goto :select

:case_9
	echo bye
	exit
