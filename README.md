# Filesystem Daemon

Un daemon de Go que monitoriza un directorio y expone una interfaz gRPC para operaciones de sistema de archivos.

## Estructura del Proyecto

```
filesystem-daemon/
├── cmd/                 # Comandos principales
├── internal/           # Código interno
│   ├── grpc/          # Implementación de gRPC
│   └── fsmonitor/     # Monitoreo del sistema de archivos
├── pkg/               # Paquetes reutilizables
└── debian/           # Archivos para el paquete .deb
```

## Instalación desde Paquete .deb

1. Añadir el repositorio:
```bash
curl -fsSL "https://windsurf-stable.codeiumdata.com/wVxQEIWkwPUEAGf3/windsurf.gpg" | sudo gpg --dearmor -o /usr/share/keyrings/filesystem-daemon-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/filesystem-daemon-archive-keyring.gpg arch=amd64] https://windsurf-stable.codeiumdata.com/wVxQEIWkwPUEAGf3/apt stable main" | sudo tee /etc/apt/sources.list.d/filesystem-daemon.list > /dev/null
```

2. Actualizar lista de paquetes:
```bash
sudo apt update
```

3. Instalar el daemon:
```bash
sudo apt install filesystem-daemon
```

## Flujo de Publicación

1. Preparar el entorno de construcción:
```bash
# Clonar el repositorio
git clone https://github.com/yourusername/filesystem-daemon.git
cd filesystem-daemon

# Instalar dependencias de construcción
sudo apt install debhelper dh-make golang
```

2. Construir el paquete:
```bash
# Construir el ejecutable de Go
go build -o cmd/filesystem-daemon

# Crear el paquete .deb
debuild -us -uc
```

3. Subir al bucket S3:
```bash
# Subir el paquete .deb
aws s3 cp ../filesystem-daemon_1.0.0_amd64.deb s3://windsurf-stable.codeiumdata.com/wVxQEIWkwPUEAGf3/apt/pool/main/

# Actualizar el índice de paquetes
aws s3 cp apt.conf s3://windsurf-stable.codeiumdata.com/wVxQEIWkwPUEAGf3/apt/conf/
aws s3 cp apt.key s3://windsurf-stable.codeiumdata.com/wVxQEIWkwPUEAGf3/apt/
```

## Configuración del Servidor S3

1. Crear bucket S3:
```bash
aws s3 mb s3://windsurf-stable.codeiumdata.com
```

2. Configurar políticas de bucket:
```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "PublicReadGetObject",
            "Effect": "Allow",
            "Principal": "*",
            "Action": "s3:GetObject",
            "Resource": "arn:aws:s3:::windsurf-stable.codeiumdata.com/*"
        }
    ]
}
```

3. Estructura del bucket:
```
windsurf-stable.codeiumdata.com/
├── apt/
│   ├── conf/
│   │   └── apt.conf
│   ├── pool/
│   │   └── main/
│   │       └── filesystem-daemon_1.0.0_amd64.deb
│   └── wVxQEIWkwPUEAGf3/
│       └── apt.key
└── wVxQEIWkwPUEAGf3/
    └── windsurf.gpg
```

## Configuración del Servicio

El daemon se instala como servicio systemd y puede ser configurado a través de archivo de configuración:

```
/etc/filesystem-daemon/config.yaml
```

## Uso

1. Iniciar el servicio:
```bash
sudo systemctl start filesystem-daemon
```

2. Verificar estado:
```bash
sudo systemctl status filesystem-daemon
```

3. Cliente gRPC (C#):
```csharp
using Grpc.Net.Client;

var channel = GrpcChannel.ForAddress("http://localhost:50051");
var client = new Filesystem.FilesystemClient(channel);
```

## Desarrollo

1. Clonar el repositorio:
```bash
git clone https://github.com/yourusername/filesystem-daemon.git
cd filesystem-daemon
```

2. Instalar dependencias:
```bash
go mod tidy
```

3. Ejecutar en modo desarrollo:
```bash
go run cmd/filesystem-daemon/main.go --watch-dir=/ruta/al/directorio
```

## Contribución

1. Fork el repositorio
2. Crear una rama para tu característica (`git checkout -b feature/amazing-feature`)
3. Commit tus cambios (`git commit -m 'Add some amazing feature'`)
4. Push a la rama (`git push origin feature/amazing-feature`)
5. Abre un Pull Request
