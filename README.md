# Flow

Flow es un CLI de task tracking con TUI (Terminal User Interface) y servidor MCP (Model Context Protocol) construido en Go.

## Características

- **Task tracking**: Gestiona tareas con estados y metadatos
- **Pomodoro timer**: Sesiones de trabajo con temporizador integrado
- **TUI interactiva**: Interfaz visual con Bubbletea
- **MCP Server**: Integración con Model Context Protocol
- **Git integration**: Detección automática de contexto git
- **SQLite storage**: Persistencia local ligera

## Arquitectura

El proyecto sigue una arquitectura hexagonal (clean architecture):

```
flow/
├── cmd/                    # Entry points (cobra)
├── internal/
│   ├── domain/            # Entidades de negocio
│   ├── ports/             # Interfaces
│   ├── services/          # Casos de uso
│   ├── adapters/          # Implementaciones
│   └── config/            # Configuración
└── pkg/                   # Librerías públicas
```

## Instalación

```bash
go install github.com/xavier/flow@latest
```

## Uso

```bash
# Agregar una tarea
flow add "Implementar feature X"

# Listar tareas
flow list

# Iniciar pomodoro en una tarea
flow start 1

# Ver estado actual
flow status

# Iniciar break
flow break

# Iniciar servidor MCP
flow mcp
```

## Comandos TUI

Durante una sesión pomodoro:
- `s` - Iniciar/Pausar timer
- `b` - Iniciar break
- `q` - Salir

## Licencia

MIT
