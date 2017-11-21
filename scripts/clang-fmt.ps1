# This requires clang-format to be installed and in %PATH%
# http://llvm.org/releases/download.html
# it's part of clang installer

Remove-Item src\*.bak, src\*.tmp

$files =
"src\*.cpp",
"src\*.h",
"src\mui\SvgPath.*",
# TODO: the rest of src\mui\
"src\utils\*.cpp",
"src\utils\*.h",
"src\utils\tests\*.cpp",
"src\utils\tests\*.h",
"src\wingui\*.cpp",
"src\wingui\*.h",
"src\installer\Install.cpp",
"src\installer\Installer.cpp",
"src\installer\Installer.h",
"src\installer\Uninstall.cpp",
"src\tools\*.cpp"
"src\tools\*.h"

foreach ($file in $files) {
  $files2 = Get-ChildItem $file
  foreach ($file2 in $files2) {
    Write-Host $file2
    clang-format.exe -i -style=file $file2
  }
}

Get-ChildItem -Recur -Filter "*.tmp" | Remove-Item
Get-ChildItem -Recur -Filter "*.bak" | Remove-Item
