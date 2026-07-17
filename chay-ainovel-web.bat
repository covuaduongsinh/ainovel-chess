@echo off
chcp 65001 >nul
setlocal

rem === Launcher: khoi dong ainovel-cli o che do Web UI ===
rem Bam dup vao file nay de build lai, tu bat cau noi Meridian, va chay phan mem tren trinh duyet.

cd /d "%~dp0"

echo ============================================
echo   AINOVEL - Khoi dong che do Web UI
echo ============================================
echo.

rem --- 0. Kiem tra Go ---
where go >nul 2>nul
if errorlevel 1 (
    echo [LOI] Khong tim thay Go tren may.
    echo Ban can cai dat Go ^(phien ban 1.25.5 tro len^) tu: https://go.dev/dl/
    echo Sau khi cai xong, mo lai file nay.
    echo.
    pause
    exit /b 1
)

rem --- 1. Build lai tu ma nguon ---
echo [1/3] Dang build tu ma nguon... (co the mat vai giay)
go build -o ainovel-cli.exe ./cmd/ainovel-cli
if errorlevel 1 (
    echo.
    echo [LOI] Build that bai. Vui long kiem tra lai ma nguon o phia tren.
    echo Khong chay ban cu de tranh nham lan.
    echo.
    pause
    exit /b 1
)
echo       Build xong.
echo.

rem --- 2. Bao dam cau noi Meridian dang chay (provider "claude-code" can no o cong 3456) ---
echo [2/3] Kiem tra cau noi Meridian (cong 3456)...
set "_MERIDIAN_UP="
curl -s -m 2 -o nul http://127.0.0.1:3456/ && set "_MERIDIAN_UP=1"
if defined _MERIDIAN_UP (
    echo       Meridian da chay san.
) else (
    where meridian >nul 2>nul && (
        echo       Dang khoi dong Meridian trong cua so rieng ^(thu nho^)...
        start "Meridian (cau noi model)" /min meridian
        call :wait_meridian
    ) || (
        echo       [CANH BAO] Chua cai Meridian. Provider "claude-code" can no de goi model.
        echo       Cai bang lenh:  npm i -g @rynfar/meridian
        echo       Van tiep tuc ^(neu ban dung provider khac nhu gemini thi khong sao^).
    )
)
echo.

rem --- 3. Chay che do Web UI ---
echo [3/3] Dang khoi dong Web UI tai http://localhost:8765
echo       Trinh duyet se tu dong mo. Dong cua so nay de tat phan mem.
echo       ^(Cua so Meridian van chay rieng; dong no neu muon tat han cau noi model.^)
echo.
"%~dp0ainovel-cli.exe" --web --port 8765

echo.
echo Phan mem da dung.
pause
endlocal
exit /b 0

rem ============================================================
rem  Chuong trinh con: cho Meridian san sang (toi da 20 giay)
rem ============================================================
:wait_meridian
setlocal enabledelayedexpansion
set /a _tries=0
:wm_loop
"%SystemRoot%\System32\ping.exe" -n 2 127.0.0.1 >nul
curl -s -m 2 -o nul http://127.0.0.1:3456/ && (
    echo       Meridian da san sang.
    endlocal & exit /b 0
)
set /a _tries+=1
if !_tries! lss 20 goto :wm_loop
echo       [CANH BAO] Meridian chua phan hoi sau 20 giay, van tiep tuc chay app...
endlocal & exit /b 0
