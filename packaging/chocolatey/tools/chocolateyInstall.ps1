$ErrorActionPreference = 'Stop'
$toolsDir = "$(Split-Path -parent $MyInvocation.MyCommand.Definition)"
$version = '0.1.0'
$url64 = "https://github.com/bkneis/lazyaws/releases/download/v${version}/lazyaws_${version}_windows_amd64.zip"

$packageArgs = @{
  packageName   = $env:ChocolateyPackageName
  unzipLocation = $toolsDir
  url64bit      = $url64
  checksum64    = 'REPLACE_WITH_SHA256'
  checksumType64= 'sha256'
}
Install-ChocolateyZipPackage @packageArgs
