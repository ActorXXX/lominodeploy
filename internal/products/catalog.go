package products

// Catalog define los productos disponibles para instalar.
// Se puede extender descargando una versión actualizada desde LominoLLS.

type Parameter struct {
	ID             string   `json:"id"`
	Label          string   `json:"label"`
	Type           string   `json:"type"` // text, email, password, select, number, boolean
	Placeholder    string   `json:"placeholder,omitempty"`
	Default        string   `json:"default,omitempty"`
	Options        []string `json:"options,omitempty"`
	Required       bool     `json:"required"`
	Generated      bool     `json:"generated"`       // genera valor automático si está vacío
	HiddenByDef    bool     `json:"hidden_by_default,omitempty"`
	Tooltip        string   `json:"tooltip,omitempty"`
	Group          string   `json:"group,omitempty"`
}

type CatalogEntry struct {
	ID             string      `json:"id"`
	Name           string      `json:"name"`
	Tagline        string      `json:"tagline"`
	Description    string      `json:"description"`
	Image          string      `json:"image"`
	MinRAMGB       float64     `json:"min_ram_gb"`
	MinDiskGB      float64     `json:"min_disk_gb"`
	RequiredPorts  []int       `json:"required_ports"`
	Parameters     []Parameter `json:"parameters"`
	DeployCommands []string    `json:"deploy_commands"`
}

// BuiltinCatalog es el catálogo embebido en el binario.
// Cada producto puede tener su versión actualizada en LominoLLS.
var BuiltinCatalog = []CatalogEntry{
	{
		ID:          "lominoisp",
		Name:        "LominoISP",
		Tagline:     "Sistema de gestión para ISPs",
		Description: "Plataforma completa para proveedores de servicios de internet. Gestión de clientes, facturación, planes, mikrotik y más.",
		Image:       "ghcr.io/lominodev/lominoisp",
		MinRAMGB:    4,
		MinDiskGB:   40,
		RequiredPorts: []int{80, 443},
		Parameters: []Parameter{
			{ID: "DOMAIN", Label: "Dominio de acceso", Type: "text", Placeholder: "app.tu-isp.com", Required: true, Tooltip: "Dominio o subdominio donde los usuarios accederán al sistema."},
			{ID: "ADMIN_EMAIL", Label: "Email del administrador", Type: "email", Placeholder: "admin@tu-isp.com", Required: true},
			{ID: "ADMIN_PASSWORD", Label: "Contraseña del administrador", Type: "password", Required: true, Generated: true, Tooltip: "Se genera automáticamente si se deja vacío."},
			{ID: "COMPANY_NAME", Label: "Nombre del ISP", Type: "text", Placeholder: "Mi ISP S.A.", Required: true},
			{ID: "TIMEZONE", Label: "Zona horaria", Type: "select", Options: []string{"America/Lima", "America/Bogota", "America/Santiago", "America/La_Paz", "America/Buenos_Aires", "America/Sao_Paulo", "America/Mexico_City", "UTC"}, Default: "America/Lima", Required: true},
			{ID: "DB_PASSWORD", Label: "Contraseña de PostgreSQL", Type: "password", Required: true, Generated: true},
			{ID: "SECRET_KEY", Label: "Django Secret Key", Type: "password", Required: true, Generated: true, HiddenByDef: true, Tooltip: "Clave secreta de Django. No cambiar después de instalar."},
			{ID: "SMTP_HOST", Label: "Servidor SMTP (opcional)", Type: "text", Placeholder: "smtp.gmail.com", Group: "Correo electrónico"},
			{ID: "SMTP_PORT", Label: "Puerto SMTP", Type: "number", Default: "587", Group: "Correo electrónico"},
			{ID: "SMTP_USER", Label: "Usuario SMTP", Type: "text", Group: "Correo electrónico"},
			{ID: "SMTP_PASSWORD", Label: "Contraseña SMTP", Type: "password", Group: "Correo electrónico"},
		},
		DeployCommands: []string{
			"docker compose pull",
			"docker compose up -d",
			"docker compose exec -T web python manage.py migrate --noinput",
			"docker compose exec -T web python manage.py collectstatic --noinput",
			"docker compose exec -T web python manage.py createsuperuser --noinput",
		},
	},
	{
		ID:          "lominoip",
		Name:        "LominoIP",
		Tagline:     "Gestión de direccionamiento IP y RADIUS",
		Description: "Sistema de gestión de bloques IP, RADIUS y AAA para redes de ISPs. Integración con Mikrotik y equipos WISP.",
		Image:       "ghcr.io/lominodev/lominoip",
		MinRAMGB:    2,
		MinDiskGB:   20,
		RequiredPorts: []int{80, 443, 1812, 1813},
		Parameters: []Parameter{
			{ID: "DOMAIN", Label: "Dominio de acceso", Type: "text", Placeholder: "radius.tu-isp.com", Required: true},
			{ID: "ADMIN_EMAIL", Label: "Email del administrador", Type: "email", Required: true},
			{ID: "ADMIN_PASSWORD", Label: "Contraseña del administrador", Type: "password", Required: true, Generated: true},
			{ID: "DB_PASSWORD", Label: "Contraseña de PostgreSQL", Type: "password", Required: true, Generated: true},
			{ID: "SECRET_KEY", Label: "Django Secret Key", Type: "password", Required: true, Generated: true, HiddenByDef: true},
			{ID: "RADIUS_SECRET", Label: "Secreto RADIUS compartido", Type: "password", Required: true, Generated: true, Tooltip: "Secreto compartido entre los NAS y el servidor RADIUS."},
		},
		DeployCommands: []string{
			"docker compose pull",
			"docker compose up -d",
			"docker compose exec -T web python manage.py migrate --noinput",
			"docker compose exec -T web python manage.py collectstatic --noinput",
			"docker compose exec -T web python manage.py createsuperuser --noinput",
		},
	},
	{
		ID:          "lominostore",
		Name:        "LominoStore",
		Tagline:     "Tienda en línea para LATAM",
		Description: "Plataforma de comercio electrónico con soporte para múltiples monedas latinoamericanas, pasarelas de pago locales y almacenamiento MinIO.",
		Image:       "ghcr.io/lominodev/lominostore",
		MinRAMGB:    4,
		MinDiskGB:   40,
		RequiredPorts: []int{80, 443},
		Parameters: []Parameter{
			{ID: "DOMAIN", Label: "Dominio de la tienda", Type: "text", Placeholder: "tienda.tu-empresa.com", Required: true},
			{ID: "STORE_NAME", Label: "Nombre de la tienda", Type: "text", Placeholder: "Mi Tienda Online", Required: true},
			{ID: "ADMIN_EMAIL", Label: "Email del administrador", Type: "email", Required: true},
			{ID: "ADMIN_PASSWORD", Label: "Contraseña del administrador", Type: "password", Required: true, Generated: true},
			{ID: "CURRENCY", Label: "Moneda", Type: "select", Options: []string{"PEN", "USD", "CLP", "COP", "ARS", "MXN", "BRL", "BOB"}, Default: "PEN"},
			{ID: "TIMEZONE", Label: "Zona horaria", Type: "select", Options: []string{"America/Lima", "America/Bogota", "America/Santiago", "America/La_Paz", "America/Buenos_Aires", "UTC"}, Default: "America/Lima"},
			{ID: "DB_PASSWORD", Label: "Contraseña de PostgreSQL", Type: "password", Required: true, Generated: true},
			{ID: "SECRET_KEY", Label: "Django Secret Key", Type: "password", Required: true, Generated: true, HiddenByDef: true},
			{ID: "MINIO_SECRET", Label: "Clave MinIO", Type: "password", Required: true, Generated: true, Tooltip: "Clave de acceso para el almacenamiento de archivos."},
		},
		DeployCommands: []string{
			"docker compose pull",
			"docker compose up -d",
			"docker compose exec -T web python manage.py migrate --noinput",
			"docker compose exec -T web python manage.py collectstatic --noinput",
			"docker compose exec -T web python manage.py createsuperuser --noinput",
			"docker compose exec -T web python manage.py setup_store",
		},
	},
}

func FindInCatalog(id string) *CatalogEntry {
	for i := range BuiltinCatalog {
		if BuiltinCatalog[i].ID == id {
			return &BuiltinCatalog[i]
		}
	}
	return nil
}
