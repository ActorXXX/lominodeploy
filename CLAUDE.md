# LominoDeploy — Sistema de Despliegue Guiado — Diseño Completo

> **Documento de diseño y planificación**  
> Proyecto: LominoDev — LominoDeploy  
> Propósito: Aplicación web transitoria de instalación guiada para productos LominoDev  
> Estado: Diseño aprobado — pendiente implementación en VS Code

---

## 1. Visión General

**LominoDeploy** reemplaza los scripts de instalación tradicionales por una
aplicación web transitoria que guía al cliente paso a paso para desplegar un producto LominoDev
en su propia infraestructura (VPS, VM o servidor dedicado).

### Principios de diseño

- **Uno por producto**: cada producto tiene su propio flujo específico con su propio flujo.
- **Transitorio**: LominoDeploy corre en un puerto temporal y se autodestruye al finalizar.
- **Autónomo**: el cliente solo necesita ejecutar un comando de una línea para iniciar.
- **Seguro**: los tokens de acceso a contenedores son temporales (1 hora) y se generan on-demand.
- **Informativo**: el cliente ve en todo momento qué se está haciendo y por qué.

---

## 2. Arquitectura del Sistema

```
┌─────────────────────────────────────────────────────────────────┐
│                    LominoLLS (license.lominodev.com)            │
│                                                                 │
│  /install/<producto>     → sirve bootstrap.sh                   │
│  /api/install/token      → genera GHCR token vía GitHub App     │
│                                                                 │
│  [ GitHub App: "LominoDeploy" ]                          │
│    App ID + Private Key (.pem) como variables de entorno        │
└──────────────────────────────┬──────────────────────────────────┘
                               │
                    curl -sSL .../install/<producto> | bash
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                    VPS / VM del Cliente                         │
│                                                                 │
│  bootstrap.sh                                                   │
│  ├── Detecta SO y arquitectura                                  │
│  ├── Instala Docker si aplica                                   │
│  ├── Descarga imagen: ghcr.io/lominodev/lominodeploy-<producto>       │
│  └── Levanta LominoDeploy en puerto :7788 (temporal)                  │
│                                                                 │
│  LominoDeploy (FastAPI + Alpine.js + Tailwind)                    │
│  ├── Step 1: System Check                                       │
│  ├── Step 2: Docker Check                                       │
│  ├── Step 3: Licencia → llama LominoLLS → obtiene GHCR token   │
│  ├── Step 4: Estructura de archivos                             │
│  ├── Step 5: Parámetros del producto                            │
│  ├── Step 6: Resumen y confirmación                             │
│  ├── Step 7: Despliegue en vivo (WebSocket log)                 │
│  └── Step 8: Finalización + autodestrucción                     │
└─────────────────────────────────────────────────────────────────┘
```

---

## 3. Bootstrap Script (`bootstrap.sh`)

Uno por producto. Alojado en LominoLLS como endpoint público.

**URL de acceso al cliente:**
```
curl -sSL https://license.lominodev.com/deploy/lominoisp | bash
curl -sSL https://license.lominodev.com/deploy/revid | bash
curl -sSL https://license.lominodev.com/deploy/lominostore | bash
```

### Responsabilidades del bootstrap.sh

```
1. Detectar SO (Ubuntu, Debian, RHEL/CentOS, etc.) y arquitectura (x86_64, arm64)
2. Verificar que corre como root o con sudo
3. Verificar conectividad a internet (ghcr.io y license.lominodev.com)
4. Si el producto requiere Docker:
   a. Verificar si Docker está instalado y la versión es compatible
   b. Si NO está instalado: ofrecer instalación automática (requiere root)
      - Ubuntu/Debian: apt-get install docker-ce
      - RHEL/CentOS: dnf install docker-ce
      - Si no puede instalar: mostrar instrucciones y salir
5. Hacer docker pull de la imagen de LominoDeploy (sin autenticación, imagen pública)
   ghcr.io/lominodev/lominodeploy-<producto>:latest
6. Levantar LominoDeploy en puerto :7788
7. Mostrar en terminal: "LominoDeploy listo → http://<IP_DETECTADA>:7788"
```

### Detección de IP para mostrar al cliente

```bash
# Orden de preferencia:
# 1. IP pública (curl ifconfig.me)
# 2. IP de interfaz principal (ip route get 1 | awk ...)
# 3. Mostrar ambas si hay NAT
```

---

## 4. Stack Técnico de LominoDeploy

| Componente | Tecnología | Justificación |
|---|---|---|
| Backend | **FastAPI** | Liviano, async nativo, WebSocket built-in, arranca en <1s |
| Frontend | **HTML + Alpine.js + Tailwind CDN** | Sin build step, todo en un solo archivo |
| Ejecución shell | `asyncio.subprocess` | Stream de logs en tiempo real |
| Comunicación real-time | **WebSocket** (FastAPI nativo) | Log en vivo del despliegue |
| Empaquetado | **Docker image** en ghcr.io | Portable, sin dependencias externas |
| Autodestrucción | Docker API socket | LominoDeploy se elimina a sí mismo al finalizar |

### Por qué FastAPI y no Django para LominoDeploy

LominoDeploy es transitorio y mínimo — no necesita ORM, admin, migrations ni apps.
FastAPI es la herramienta correcta para este propósito específico.
Django es el stack correcto para LominoLLS y los productos finales.

---

## 5. Flujo Detallado de Steps — Template Base

### STEP 1 — System Check

**Propósito:** Verificar que el servidor cumple los requisitos antes de continuar.

Checks a realizar:
```
✅ Sistema operativo: nombre, versión, arquitectura
✅ RAM total disponible vs. RAM mínima requerida por el producto
✅ Espacio en disco disponible (en /opt o ruta de instalación) vs. mínimo requerido
✅ Puertos requeridos por el producto (80, 443, 5432, etc.) — ¿libres o en uso?
✅ Conectividad a ghcr.io (pull de imagen)
✅ Conectividad a license.lominodev.com (validación de licencia)
✅ Usuario actual: ¿tiene permisos sudo/root?
```

Resultado posible:
- Todo OK → botón "Continuar" habilitado
- Advertencias (ej: RAM justa) → continuar con aviso
- Error crítico (ej: disco insuficiente) → bloquear con mensaje y solución sugerida

---

### STEP 2 — Docker Check (condicional por producto)

**Aplica solo si el producto requiere Docker** (flag en la configuración del producto).

```
¿Docker instalado?
├── SÍ
│   ├── ¿Versión compatible? (≥ 24.0 recomendado)
│   │   ├── SÍ → ✅ continúa
│   │   └── NO → advertencia + enlace para actualizar
│   └── ¿Docker Compose disponible? (docker compose v2)
│       ├── SÍ → ✅ continúa
│       └── NO → instrucciones para instalar plugin
└── NO
    ├── Opción A: [Instalar Docker automáticamente]
    │   → LominoDeploy ejecuta script oficial de Docker para el SO detectado
    │   → Log en vivo de la instalación
    │   → Verifica instalación exitosa
    └── Opción B: [Ver instrucciones manuales]
        → Muestra comandos específicos para el SO del servidor
        → Botón "Ya lo instalé, verificar de nuevo"
```

---

### STEP 3 — Validación de Licencia y Obtención de Token GHCR

**Propósito:** Validar que el cliente tiene una licencia activa y obtener acceso al repositorio de contenedores.

```
Cliente ingresa: License Key (formato: LDEV-XXXX-XXXX-XXXX-XXXX)
        ↓
LominoDeploy → POST https://license.lominodev.com/api/install/token
  Body: { license_key, product, server_hostname, server_ip }
        ↓
LominoLLS:
  1. Valida que la license_key existe y está activa
  2. Valida que el producto coincide con la licencia
  3. Valida que no ha expirado
  4. Llama a GitHub API con credenciales de GitHub App
     → genera Installation Access Token (válido 1 hora)
  5. Registra el evento de instalación (IP, fecha, hostname)
  6. Responde:
     {
       "valid": true,
       "ghcr_token": "ghs_xxxxxxxxxxxx",
       "image": "ghcr.io/lominodev/lominoisp",
       "tag": "latest",
       "client_name": "ISP Ejemplo S.A.",
       "license_type": "volumen",
       "max_subscribers": 500,
       "expires_at": "2026-01-01"
     }
        ↓
LominoDeploy: docker login ghcr.io -u lominodev-installer -p <ghcr_token>
        ↓
✅ Acceso confirmado — token expira en 1h (suficiente para el pull)
```

Errores manejados:
- Licencia no encontrada
- Licencia expirada
- Producto no coincide
- Licencia ya instalada en otro servidor (advertencia)
- LominoLLS no accesible (timeout)

---

### STEP 4 — Estructura de Archivos

**Propósito:** Crear los directorios y archivos base que necesita la aplicación antes de configurarla.

```
Ruta base por defecto: /opt/lominodev/<producto>/
  ├── docker-compose.yml    (generado por LominoDeploy)
  ├── .env                  (generado con los parámetros del Step 5)
  ├── data/                 (volúmenes persistentes)
  │   ├── postgres/
  │   └── redis/
  ├── media/                (archivos subidos por usuarios)
  ├── static/               (archivos estáticos)
  └── logs/                 (logs de la aplicación)
```

Acciones:
- Crear estructura con permisos correctos
- Generar `docker-compose.yml` base para el producto (plantilla incluida en la imagen de LominoDeploy)
- El archivo `.env` se genera vacío y se llena en el Step 5
- Mostrar árbol de directorios creados al cliente

El cliente puede cambiar la ruta base antes de proceder.

---

### STEP 5 — Parámetros del Producto

**Propósito:** Recopilar toda la configuración específica del producto para generar el `.env`.

Los campos varían por producto (ver Sección 6).

Características del formulario:
- Validación en tiempo real (formato de dominio, contraseñas seguras, etc.)
- Campos con valores por defecto inteligentes (ej: genera password seguro automáticamente)
- Tooltips explicativos para clientes semi-técnicos
- Opción de mostrar/ocultar contraseñas
- Al completar: genera el `.env` y lo escribe en disco

---

### STEP 6 — Resumen y Confirmación

**Propósito:** Mostrar un resumen completo de todo lo que se va a ejecutar antes de proceder.

Muestra:
```
📋 RESUMEN DE INSTALACIÓN
─────────────────────────────────────
Producto:        LominoISP v2.1.0
Cliente:         ISP Ejemplo S.A.
Licencia:        LDEV-XXXX-XXXX ✅
Servidor:        Ubuntu 22.04 | 8GB RAM | 100GB disco

Imagen Docker:   ghcr.io/lominodev/lominoisp:2.1.0
Ruta:            /opt/lominodev/lominoisp/
Dominio:         app.ispdemo.com
Base de datos:   PostgreSQL 16 (contenedor)
Puerto HTTP:     80 / 443

Comandos que se ejecutarán:
  docker compose pull
  docker compose up -d
  docker compose exec web python manage.py migrate
  docker compose exec web python manage.py collectstatic
  docker compose exec web python manage.py createsuperuser --noinput
─────────────────────────────────────
[◀ Editar parámetros]    [▶ Iniciar despliegue]
```

---

### STEP 7 — Despliegue en Vivo

**Propósito:** Ejecutar el despliegue y mostrar el log en tiempo real.

```
Comunicación: WebSocket entre el backend FastAPI y el frontend
Proceso:
  1. docker compose pull        (puede tomar varios minutos)
  2. docker compose up -d
  3. Esperar health checks de contenedores
  4. docker compose exec web python manage.py migrate
  5. docker compose exec web python manage.py collectstatic --noinput
  6. Comandos específicos del producto (ej: seed data inicial)
  7. Verificación final: HTTP GET al dominio/IP configurado
```

UI durante el despliegue:
- Terminal-style log en vivo (texto oscuro, fuente monospace)
- Barra de progreso por etapas
- Cada línea con timestamp
- Errores en rojo con sugerencia de solución
- El cliente NO puede navegar hacia atrás durante el despliegue

En caso de error:
- Mostrar el error exacto del log
- Ofrecer opciones: Reintentar desde el punto de falla / Ver log completo / Contactar soporte

---

### STEP 8 — Finalización y Autodestrucción

**Propósito:** Confirmar despliegue exitoso, entregar accesos y eliminar LominoDeploy.

Muestra:
```
✅ ¡LominoISP desplegado exitosamente!

🌐 URL de acceso:     https://app.ispdemo.com
👤 Usuario admin:     admin@ispdemo.com
🔑 Contraseña:        [generada — copiar ahora, no se mostrará de nuevo]

📄 Archivos de configuración:
   /opt/lominodev/lominoisp/.env
   /opt/lominodev/lominoisp/docker-compose.yml

🔄 Para futuras actualizaciones:
   Desde el panel de LominoISP → Configuración → Actualizar sistema
   O manualmente: cd /opt/lominodev/lominoisp && docker compose pull && docker compose up -d

⚠️  LominoDeploy se eliminará automáticamente en 60 segundos.
    [Mantener activo]    [Eliminar ahora]
```

Proceso de autodestrucción:
```bash
# LominoDeploy ejecuta sobre sí mismo:
docker stop lominodeploy && docker rm lominodeploy
# El puerto :7788 queda liberado
```

---

## 6. Parámetros por Producto (Step 5)

### 6.1 LominoISP

| Parámetro | Tipo | Descripción | Defecto |
|---|---|---|---|
| `DOMAIN` | texto | Dominio o subdominio de acceso | — |
| `DB_PASSWORD` | password | Contraseña de PostgreSQL | generada |
| `SECRET_KEY` | texto | Django Secret Key | generada |
| `ADMIN_EMAIL` | email | Email del superusuario inicial | — |
| `ADMIN_PASSWORD` | password | Contraseña del superusuario | generada |
| `COMPANY_NAME` | texto | Nombre del ISP (branding) | — |
| `TIMEZONE` | select | Zona horaria del servidor | America/Lima |
| `ENABLE_SSL` | bool | Configurar SSL con Let's Encrypt | true |
| `SMTP_HOST` | texto | Servidor SMTP para notificaciones | — |
| `SMTP_PORT` | número | Puerto SMTP | 587 |
| `SMTP_USER` | texto | Usuario SMTP | — |
| `SMTP_PASSWORD` | password | Contraseña SMTP | — |

Validaciones específicas:
- Dominio debe resolver a la IP del servidor (verificación DNS en tiempo real)
- Si `ENABLE_SSL=true`, verificar que puertos 80 y 443 estén accesibles desde internet

---

### 6.2 Revid

| Parámetro | Tipo | Descripción | Defecto |
|---|---|---|---|
| `DOMAIN` | texto | Dominio o IP de acceso | — |
| `DB_PASSWORD` | password | Contraseña de PostgreSQL | generada |
| `SECRET_KEY` | texto | Django Secret Key | generada |
| `ADMIN_EMAIL` | email | Email del superusuario | — |
| `ADMIN_PASSWORD` | password | Contraseña del superusuario | generada |
| `GPU_DEVICE` | select | Dispositivo GPU (Intel Arc A380, VAAPI, CPU fallback) | auto-detectado |
| `OUTPUT_BACKEND` | select | Backend de salida (Flussonic, Wowza, Nginx RTMP) | — |
| `OUTPUT_URL` | texto | URL del backend de salida | — |
| `MAX_CHANNELS` | número | Canales de transcodificación simultáneos | según licencia |
| `STORAGE_PATH` | texto | Ruta para archivos temporales de transcodificación | /opt/revid/data |

Checks adicionales en System Check para Revid:
- Detectar GPU Intel Arc disponible (`lspci | grep -i arc`)
- Verificar drivers QSV/VAAPI instalados
- Si no hay GPU: advertencia — modo CPU fallback con rendimiento limitado

---

### 6.3 LominoStore

| Parámetro | Tipo | Descripción | Defecto |
|---|---|---|---|
| `DOMAIN` | texto | Dominio de la tienda | — |
| `DB_PASSWORD` | password | Contraseña de PostgreSQL | generada |
| `SECRET_KEY` | texto | Django Secret Key | generada |
| `ADMIN_EMAIL` | email | Email del superusuario | — |
| `ADMIN_PASSWORD` | password | Contraseña del superusuario | generada |
| `STORE_NAME` | texto | Nombre de la tienda | — |
| `CURRENCY` | select | Moneda (PEN, USD, CLP, COP, etc.) | PEN |
| `TIMEZONE` | select | Zona horaria | America/Lima |
| `ENABLE_SSL` | bool | SSL con Let's Encrypt | true |
| `MINIO_SECRET` | password | Clave de acceso MinIO (almacenamiento) | generada |

---

## 7. Integración con LominoLLS — GitHub App

### 7.1 Registro de la GitHub App (una sola vez)

```
GitHub.com → Settings → Developer settings → GitHub Apps → New GitHub App

Nombre:          LominoDeploy
Homepage URL:    https://lominodev.com
Permisos:
  - Packages: Read        (acceso a ghcr.io)
  - Contents: Read        (acceso a repositorios privados si aplica)
Webhook:         desactivado
Instalación:     Solo cuenta LominoDev (not public)

Al crear:
  → Anotar App ID
  → Generar Private Key → descargar .pem
  → Instalar la App en la organización/cuenta lominodev
  → Anotar Installation ID
```

### 7.2 Variables de entorno en LominoLLS

```env
GITHUB_APP_ID=123456
GITHUB_APP_INSTALLATION_ID=78901234
GITHUB_APP_PRIVATE_KEY="-----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----"
```

### 7.3 Servicio en LominoLLS (`github_token_service.py`)

```
Ubicación: lominolls/apps/licensing/services/github_token_service.py

Función principal: generate_ghcr_token()
  1. Genera JWT firmado con Private Key + App ID (válido 10 min)
  2. POST https://api.github.com/app/installations/{id}/access_tokens
     Headers: Authorization: Bearer <jwt>
  3. Retorna Installation Access Token (válido 1 hora)
  4. Token se usa para: docker login ghcr.io -u x -p <token>

Dependencias Python:
  - PyJWT
  - cryptography
  - httpx (o requests)
```

### 7.4 Endpoint en LominoLLS

```
POST /api/install/token
Auth: ninguna (el license_key es la autenticación)

Request:
{
  "license_key": "LDEV-XXXX-XXXX-XXXX-XXXX",
  "product": "lominoisp",
  "server_ip": "190.x.x.x",
  "server_hostname": "vps-cliente.ejemplo.com"
}

Response OK:
{
  "valid": true,
  "ghcr_token": "ghs_xxxxxxxxxxxxxxxxxxxx",
  "image": "ghcr.io/lominodev/lominoisp",
  "tag": "2.1.0",
  "client_name": "ISP Ejemplo S.A.",
  "license_type": "volumen",
  "expires_at": "2026-12-31"
}

Response ERROR:
{
  "valid": false,
  "error": "license_expired",
  "message": "La licencia LDEV-XXXX expiró el 2025-06-01"
}
```

---

## 8. Estructura del Repositorio lominodev-deploy

```
lominodev-deploy/                    (repositorio en GitHub, privado)
├── base/                             (código compartido entre todos los productos)
│   ├── lominodeploy/
│   │   ├── main.py                   (FastAPI app base)
│   │   ├── steps/
│   │   │   ├── system_check.py
│   │   │   ├── docker_check.py
│   │   │   ├── license.py
│   │   │   ├── files.py
│   │   │   ├── deploy.py
│   │   │   └── finalize.py
│   │   ├── templates/
│   │   │   └── index.html            (UI Alpine.js + Tailwind)
│   │   └── requirements.txt
│   └── Dockerfile.base
│
├── products/
│   ├── lominoisp/
│   │   ├── config.py                 (parámetros, requisitos, flags)
│   │   ├── compose.yml.j2            (template docker-compose.yml)
│   │   ├── env.j2                    (template .env)
│   │   ├── deploy_steps.py           (comandos post-up específicos)
│   │   └── Dockerfile                (FROM base, agrega config del producto)
│   ├── revid/
│   │   └── ... (misma estructura)
│   └── lominostore/
│       └── ... (misma estructura)
│
└── bootstrap/
    ├── lominoisp.sh
    ├── revid.sh
    └── lominostore.sh
```

---

## 9. Imagen Docker de LominoDeploy

```dockerfile
# Dockerfile (por producto, ejemplo: lominoisp)
FROM python:3.12-slim

# Instalar Docker CLI (para ejecutar docker compose desde LominoDeploy)
RUN apt-get update && apt-get install -y docker.io curl && rm -rf /var/lib/apt/lists/*

WORKDIR /lominodeploy

COPY base/lominodeploy/requirements.txt .
RUN pip install -r requirements.txt

COPY base/lominodeploy/ .
COPY products/lominoisp/config.py ./product_config.py
COPY products/lominoisp/compose.yml.j2 ./templates/
COPY products/lominoisp/env.j2 ./templates/
COPY products/lominoisp/deploy_steps.py ./

ENV PRODUCT=lominoisp
ENV PORT=7788

CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "7788"]
```

LominoDeploy necesita acceso al socket de Docker del host para ejecutar `docker compose`:
```bash
docker run -d \
  --name lominodeploy \
  -p 7788:7788 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /opt:/opt \
  ghcr.io/lominodev/lominodeploy-lominoisp:latest
```

---

## 10. Publicación en LominoLLS

Los bootstrap scripts se sirven como endpoints públicos desde LominoLLS:

```python
# En Django (LominoLLS)
# apps/installer/views.py

class BootstrapScriptView(View):
    def get(self, request, product):
        valid_products = ['lominoisp', 'revid', 'lominostore']
        if product not in valid_products:
            return Http404()
        
        # Sirve el script desde /static/bootstrap/<product>.sh
        # O lo genera dinámicamente con la URL de LominoLLS embebida
        script_path = settings.BASE_DIR / f'installer/bootstrap/{product}.sh'
        content = script_path.read_text()
        
        # Reemplaza placeholder con URL actual de LominoLLS
        content = content.replace('__LOMINODEV_URL__', settings.LOMINODEV_URL)
        
        return HttpResponse(content, content_type='text/plain')

# urls.py
path('install/<str:product>', BootstrapScriptView.as_view()),
```

---

## 11. Consideraciones de Seguridad

| Riesgo | Mitigación |
|---|---|
| GHCR token filtrado | Token válido solo 1 hora — expira antes de poder usarse de forma maliciosa |
| LominoDeploy expuesto en internet | Solo corre en puerto temporal; autodestrucción al finalizar |
| Acceso al socket de Docker | El LominoDeploy corre en la propia VPS del cliente — es su infraestructura |
| Bootstrap script modificado en tránsito | Servir siempre por HTTPS desde LominoLLS |
| License key robada | LominoLLS registra IP de instalación — detectar instalaciones duplicadas |
| .env con credenciales | Permisos 600 en el .env generado (`chmod 600 /opt/.../. env`) |

---

## 12. Roadmap de Implementación

### Fase 1 — Infraestructura base
- [ ] Crear repositorio `lominodev-deploy` en GitHub
- [ ] Registrar GitHub App "LominoDeploy" en cuenta LominoDev
- [ ] Implementar `github_token_service.py` en LominoLLS
- [ ] Implementar endpoint `POST /api/install/token` en LominoLLS
- [ ] Implementar endpoint `GET /install/<producto>` en LominoLLS

### Fase 2 — LominoDeploy base
- [ ] Implementar FastAPI app con estructura de steps
- [ ] Implementar UI (HTML + Alpine.js + Tailwind)
- [ ] Implementar WebSocket para log en vivo
- [ ] Implementar Step 1: System Check
- [ ] Implementar Step 2: Docker Check
- [ ] Implementar Step 3: Validación de licencia
- [ ] Implementar Step 4: Estructura de archivos
- [ ] Implementar Step 6: Resumen
- [ ] Implementar Step 7: Despliegue
- [ ] Implementar Step 8: Finalización + autodestrucción

### Fase 3 — Producto LominoISP
- [ ] `config.py` de LominoISP (requisitos, flags, parámetros)
- [ ] Template `docker-compose.yml.j2` para LominoISP
- [ ] Template `.env.j2` para LominoISP
- [ ] `deploy_steps.py` para LominoISP (migrate, collectstatic, createsuperuser)
- [ ] Dockerfile del LominoDeploy LominoISP
- [ ] `bootstrap/lominoisp.sh`
- [ ] Push imagen a `ghcr.io/lominodev/lominodeploy-lominoisp`
- [ ] Prueba end-to-end en VPS limpia

### Fase 4 — Productos adicionales
- [ ] Revid LominoDeploy (incluye detección de GPU)
- [ ] LominoStore LominoDeploy

---

## 13. Glosario

| Término | Definición |
|---|---|
| **Bootstrap script** | Script bash de una línea que instala el LominoDeploy |
| **LominoDeploy** | Aplicación web FastAPI transitoria que guía el despliegue |
| **LominoLLS** | License Server de LominoDev (Django) en `license.lominodev.com` |
| **GitHub App** | Entidad registrada en GitHub que permite generar tokens de acceso vía API |
| **Installation Access Token** | Token temporal (1h) generado por la GitHub App para acceder a ghcr.io |
| **ghcr.io** | GitHub Container Registry — donde se alojan las imágenes Docker de LominoDev |
| **PAT** | Personal Access Token — reemplazado en este diseño por tokens de GitHub App |
| **docker-compose.yml.j2** | Template Jinja2 del archivo docker-compose.yml del producto |
| **Autodestrucción** | El LominoDeploy elimina su propio container al finalizar el despliegue |
