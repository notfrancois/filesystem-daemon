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

Primero, instale los paquetes NuGet necesarios:
```bash
dotnet add package Grpc.Net.Client
dotnet add package Google.Protobuf
dotnet add package Grpc.Tools
```

Agregue el archivo proto a su proyecto (filesystem.proto) y configure su archivo .csproj para generar el código:
```xml
<ItemGroup>
  <Protobuf Include="filesystem.proto" GrpcServices="Client" />
</ItemGroup>
```

Ejemplo de cliente C# para operaciones de sistema de archivos:
```csharp
using System;
using System.IO;
using System.Threading.Tasks;
using Grpc.Net.Client;
using Google.Protobuf;
using Filesystem; // Namespace generado desde el proto

public class FilesystemClient
{
    private readonly FilesystemService.FilesystemServiceClient _client;

    public FilesystemClient(string serverAddress)
    {
        // Configurar el cliente con TLS para conexiones seguras
        var channel = GrpcChannel.ForAddress(serverAddress, new GrpcChannelOptions
        {
            HttpHandler = new HttpClientHandler
            {
                ServerCertificateCustomValidationCallback = HttpClientHandler.DangerousAcceptAnyServerCertificateValidator
            }
        });
        _client = new FilesystemService.FilesystemServiceClient(channel);
    }

    // Listar contenido de un directorio
    public async Task<ListResponse> ListDirectoryAsync(string path, bool recursive = false, string pattern = "")
    {
        var request = new ListRequest
        {
            Path = path,
            Recursive = recursive,
            Pattern = pattern
        };
        return await _client.ListDirectoryAsync(request);
    }

    // Obtener información de un archivo
    public async Task<FileInfo> GetFileInfoAsync(string path)
    {
        var request = new FileRequest { Path = path };
        return await _client.GetFileInfoAsync(request);
    }

    // Crear un directorio
    public async Task<OperationResponse> CreateDirectoryAsync(string path, int permissions = 0755)
    {
        var request = new CreateDirectoryRequest
        {
            Path = path,
            Permissions = permissions
        };
        return await _client.CreateDirectoryAsync(request);
    }

    // Eliminar archivo o directorio
    public async Task<OperationResponse> DeleteAsync(string path, bool recursive = false)
    {
        var request = new DeleteRequest
        {
            Path = path,
            Recursive = recursive
        };
        return await _client.DeleteAsync(request);
    }

    // Copiar archivo o directorio
    public async Task<OperationResponse> CopyAsync(string source, string destination, bool overwrite = false)
    {
        var request = new CopyRequest
        {
            Source = source,
            Destination = destination,
            Overwrite = overwrite
        };
        return await _client.CopyAsync(request);
    }

    // Mover/renombrar archivo o directorio
    public async Task<OperationResponse> MoveAsync(string source, string destination, bool overwrite = false)
    {
        var request = new MoveRequest
        {
            Source = source,
            Destination = destination,
            Overwrite = overwrite
        };
        return await _client.MoveAsync(request);
    }

    // Subir un archivo (streaming)
    public async Task<OperationResponse> UploadFileAsync(string localFilePath, string remoteFilePath)
    {
        using var call = _client.UploadFile();
        using var fileStream = new FileStream(localFilePath, FileMode.Open, FileAccess.Read);
        
        var buffer = new byte[64 * 1024]; // 64KB chunks
        int bytesRead;
        long offset = 0;
        
        while ((bytesRead = await fileStream.ReadAsync(buffer, 0, buffer.Length)) > 0)
        {
            await call.RequestStream.WriteAsync(new FileChunk
            {
                FilePath = remoteFilePath,
                Content = ByteString.CopyFrom(buffer, 0, bytesRead),
                Offset = offset,
                IsLast = bytesRead < buffer.Length
            });
            
            offset += bytesRead;
        }
        
        // Marcar el último chunk si el archivo está vacío o no terminó exactamente en el límite de buffer
        if (offset == 0 || bytesRead == buffer.Length)
        {
            await call.RequestStream.WriteAsync(new FileChunk
            {
                FilePath = remoteFilePath,
                Content = ByteString.Empty,
                Offset = offset,
                IsLast = true
            });
        }
        
        await call.RequestStream.CompleteAsync();
        return await call.ResponseAsync;
    }

    // Descargar un archivo (streaming)
    public async Task DownloadFileAsync(string remoteFilePath, string localFilePath)
    {
        var request = new FileRequest { Path = remoteFilePath };
        using var call = _client.DownloadFile(request);
        using var fileStream = new FileStream(localFilePath, FileMode.Create, FileAccess.Write);
        
        await foreach (var chunk in call.ResponseStream.ReadAllAsync())
        {
            await fileStream.WriteAsync(chunk.Content.ToByteArray(), 0, chunk.Content.Length);
        }
    }
}
```

Ejemplo de uso del cliente:
```csharp
class Program
{
    static async Task Main(string[] args)
    {
        var client = new FilesystemClient("https://localhost:50051");
        
        // Listar archivos en un directorio
        var files = await client.ListDirectoryAsync("/var/www/html");
        foreach (var file in files.Items)
        {
            Console.WriteLine($"{file.Name} - {(file.IsDirectory ? "Directory" : "File")} - {file.Size} bytes");
        }
        
        // Crear un directorio
        var createResponse = await client.CreateDirectoryAsync("/var/www/html/new-folder");
        Console.WriteLine($"Directory created: {createResponse.Success}");
        
        // Subir un archivo
        var uploadResponse = await client.UploadFileAsync("/path/to/local/file.txt", "/var/www/html/file.txt");
        Console.WriteLine($"File uploaded: {uploadResponse.Success}");
    }
}
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
