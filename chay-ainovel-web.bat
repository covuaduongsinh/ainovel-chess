@echo off
chcp 65001 >nul
setlocal

rem === Launcher: khoi dong ainovel-cli o che do Web UI ===
rem Bam dup vao file nay de build lai va chay phan mem tren trinh duyet.

cd /d "%~dp0"

echo ============================================
echo   AINOVEL - Khoi dong che do Web UI
echo ============================================
echo.

rem --- 1. Kiem tra Go ---
where go >nul 2>nul
if errorlevel 1 (
    echo [LOI] Khong tim thay Go tren may.
    echo Ban can cai dat Go ^(phien ban 1.25.5 tro len^) tu: https://go.dev/dl/
    echo Sau khi cai xong, mo lai file nay.
    echo.
    pause
    exit /b 1
)

rem --- 2. Build lai tu ma nguon ---
echo [1/2] Dang build tu ma nguon... (co the mat vai giay)
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

rem --- 3. Chay che do Web UI ---
echo [2/2] Dang khoi dong Web UI tai http://localhost:8765
echo       Trinh duyet se tu dong mo. Dong cua so nay de tat phan mem.
echo.
ainovel-cli.exe --web --port 8765

echo.
echo Phan mem da dung.
pause
endlocal
