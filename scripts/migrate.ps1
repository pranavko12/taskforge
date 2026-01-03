Get-ChildItem .\migrations\*.sql | ForEach-Object {
  Get-Content $_ | docker exec -i taskforge-postgres-1 psql -U taskforge -d taskforge
}
