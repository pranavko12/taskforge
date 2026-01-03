param (
  [string]$Service = "api"
)

docker compose logs -f $Service
