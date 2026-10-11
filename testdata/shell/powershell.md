# PowerShell

```powershell
PS> Get-ChildItem -Path .\src
PS C:\Users\me\project> npm install
added 12 packages
PS> Write-Host "hello world"
>> -ForegroundColor Green
PS> $env:TOKEN = "abc"
PS> Write-Host
>> $env:TOKEN
```

```cmd
C:\project> dir /b
src
C:\project>npm test
```

```pwsh
# build everything
Set-Location src
npm run build
```
