variable "turso_token" {
  type    = string
}

data "external_schema" "gorm" {
  program = [
    "go",
    "run",
    "-mod=mod",
    "./internal/migrate/main.go",
  ]
}

env "turso" {
  src = data.external_schema.gorm.url
  url = "libsql://flock-tanexy.aws-ap-south-1.turso.io?authToken=${var.turso_token}"
  dev = "sqlite://file?mode=memory&_fk=1"

  migration {
    dir = "file://migrations"
  }
}
