# Reporte de Testing - Flow CLI

Fecha: 2026-02-15
Proyecto: Flow CLI (Pomodoro timer con TUI y MCP server)

## Resumen Ejecutivo

El proyecto Flow CLI está bien estructurado siguiendo principios de arquitectura hexagonal/clean architecture. Los tests pasan correctamente y el código compila sin errores. Sin embargo, se encontraron varios bugs, code smells y oportunidades de mejora que deben ser atendidos.

---

## 🐛 BUGS ENCONTRADOS

### Prioridad: CRÍTICA

#### 1. `status` sin `--json` muestra "Loading..."
**Archivo:** `cmd/status.go` y `internal/adapters/tui/model.go`
**Problema:** El comando `flow status` sin la flag `--json` muestra "Loading..." porque el modelo TUI no recibe las dimensiones de la terminal.
**Causa:** En `model.go`, el método `View()` retorna "Loading..." si `m.width == 0`.
**Solución:** Usar una vista alternativa para `status` que no requiera TUI interactivo, o inicializar width/height con valores por defecto.

```go
// En model.go, línea 88
if m.width == 0 {
    return "Loading..."
}
```

**Impacto:** ALTO - El comando básico de status no funciona correctamente en modo texto.

---

### Prioridad: ALTA

#### 2. Inconsistencia en serialización JSON de tags vacíos
**Archivo:** `internal/adapters/storage/task_repository.go`
**Problema:** Cuando una tarea se crea sin tags, el JSON devuelve `null` en lugar de `[]`.
**Código problemático:**
```go
// En scanTasks() - tagsStr es ""
if tagsStr != "" {
    task.Tags = strings.Split(tagsStr, ",")
}
// Si tagsStr es "", task.Tags permanece nil
```
**Solución:** Inicializar siempre `task.Tags = []string{}` antes del if.

#### 3. Falta de manejo de errores en callbacks del TUI
**Archivo:** `cmd/start.go`, `cmd/break.go`
**Problema:** Los callbacks del TUI ignoran errores:
```go
timer.SetCommandCallback(func(cmd ports.TimerCommand) {
    switch cmd {
    case ports.CmdPause:
        _, _ = pomodoroSvc.PauseSession(ctx)  // Error ignorado!
    case ports.CmdResume:
        _, _ = pomodoroSvc.ResumeSession(ctx) // Error ignorado!
    ...
    }
})
```
**Impacto:** Los errores en operaciones de pausa/resume/cancel no se muestran al usuario.

#### 4. Goroutine leak en TUI
**Archivo:** `internal/adapters/tui/timer.go`
**Problema:** En `SetCommandCallback()`, se lanza una goroutine sin mecanismo de cancelación:
```go
func (t *Timer) SetCommandCallback(callback func(cmd ports.TimerCommand)) {
    go func() {
        for cmd := range t.cmdChan {  // Nunca termina!
            callback(cmd)
        }
    }()
}
```
**Impacto:** Cada vez que se inicia el TUI, se crea una goroutine que nunca termina.

---

### Prioridad: MEDIA

#### 5. `fmt.Scanln` puede fallar con entradas largas
**Archivo:** `cmd/stop.go`
**Problema:** Usar `fmt.Scanln` para leer notas es problemático porque se queda con la primera palabra:
```go
var notes string
fmt.Scanln(&notes)  // Solo lee hasta el espacio!
```
**Solución:** Usar `bufio.NewReader`:
```go
reader := bufio.NewReader(os.Stdin)
notes, _ := reader.ReadString('\n')
```

#### 6. No hay comando `delete` para tareas
**Archivo:** `cmd/`
**Problema:** Aunque el repositorio implementa `Delete()`, no hay comando CLI expuesto.
**Servicio sí tiene:** `taskService.DeleteTask()`
**Impacto:** Los usuarios no pueden eliminar tareas desde CLI.

#### 7. Falta validación de duración en MCP start_pomodoro
**Archivo:** `internal/adapters/mcp/mcp_server.go`
**Problema:** No se valida que `duration_minutes` sea positivo:
```go
if d := request.GetFloat("duration_minutes", 0); d > 0 {
    m := int(d)
    durationMinutes = &m
}
```
**Problema:** Valores negativos o cero podrían causar comportamiento inesperado.

#### 8. Posible race condition en TUI
**Archivo:** `internal/adapters/tui/timer.go`
**Problema:** El método `UpdateState()` puede ser llamado desde múltiples goroutines sin sincronización:
```go
func (t *Timer) UpdateState(state *domain.CurrentState) {
    if t.program != nil {
        t.program.Send(state)  // No hay sincronización
    }
}
```

---

## 🔧 CODE SMELLS Y MEJORAS

### Prioridad: ALTA

#### 9. Falta de cobertura de tests en comandos CLI
**Archivos:** `cmd/*.go`
**Problema:** No hay tests para ningún comando CLI (excepto mediante tests de integración implícita).
**Recomendación:** Extraer la lógica de los comandos a funciones testeables o usar `cobra.Command.SetArgs()` para testing.

#### 10. Manejo inconsistente de context cancellation
**Archivo:** `internal/adapters/tui/timer.go`
**Problema:** El context se usa para cancelar, pero no hay cleanup adecuado:
```go
go func() {
    <-ctx.Done()
    if t.program != nil {
        t.program.Quit()
    }
}()
```
**Mejora:** Usar `sync.WaitGroup` para esperar a que terminen las goroutines.

#### 11. No hay manejo de señales en el MCP server
**Archivo:** `internal/adapters/mcp/mcp_server.go`
**Problema:** El servidor MCP no maneja señales de sistema (SIGINT, SIGTERM) para graceful shutdown.

---

### Prioridad: MEDIA

#### 12. Uso de punteros innecesarios
**Archivo:** `internal/domain/session.go`
**Problema:** `TaskID *string` podría ser simplemente `string` con validación de vacío.
**Justificación:** Los punteros a tipos básicos aumentan la complejidad y el riesgo de nil pointer dereference.

#### 13. Funciones de ayuda duplicadas
**Archivo:** `cmd/status.go` y `internal/adapters/mcp/mcp_server.go`
**Problema:** Ambos archivos tienen lógica similar para convertir estado a JSON.
**Recomendación:** Crear un presenter/formatter compartido.

#### 14. Falta de documentación de errores
**Archivo:** `internal/domain/task.go`, `internal/domain/session.go`
**Problema:** Los errores de dominio no tienen documentación sobre cuándo ocurren.

#### 15. Inyección de dependencias manual en lugar de usar wire/dig
**Archivo:** `cmd/root.go`
**Problema:** La inicialización manual de servicios es propensa a errores:
```go
func initializeServices() error {
    // Código de inicialización muy largo...
}
```
**Recomendación:** Considerar usar `google/wire` para inyección de dependencias compile-time.

---

### Prioridad: BAJA

#### 16. No se usa el parámetro `limit` en GetRecentSessions correctamente
**Archivo:** `internal/services/pomodoro_service.go`
**Código:**
```go
func (s *PomodoroService) GetRecentSessions(ctx context.Context, limit int) ([]*domain.PomodoroSession, error) {
    since := time.Now().AddDate(0, 0, -7)  // Hardcoded 7 días!
    sessions, err := s.storage.Sessions().FindRecent(ctx, since)
    // ...
}
```
**Problema:** El límite de tiempo (7 días) está hardcodeado, no es configurable.

#### 17. Nombres de variables inconsistentes
**Archivo:** `cmd/root.go`
**Problema:** `taskService` vs `pomodoroSvc` vs `stateService` - inconsistencia en sufijos.

#### 18. No hay métricas ni observabilidad
**Problema:** No hay logging estructurado, métricas, ni tracing.
**Recomendación:** Agregar logging con `slog` y métricas opcionales.

---

## 🚀 FEATURES FALTANTES

### Prioridad: ALTA

#### 19. No hay comando `edit` para modificar tareas
Los usuarios no pueden editar el título, descripción o tags de una tarea existente.

#### 20. No hay soporte para múltiples configuraciones de pomodoro
No se pueden definir diferentes perfiles (ej: "trabajo": 50/10, "código": 25/5).

#### 21. No hay export/import de datos
No hay forma de respaldar o migrar datos de la base de datos SQLite.

---

### Prioridad: MEDIA

#### 22. No hay historial de tareas completadas en TUI
El TUI no muestra estadísticas históricas más allá del día actual.

#### 23. No hay integración con calendario/sistema de notificaciones nativo
Las notificaciones usan `beeep` pero no se integran con el centro de notificaciones del sistema.

#### 24. No hay soporte para tareas recurrentes/hábitos
No se pueden definir tareas que se repiten diariamente/semanalmente.

#### 25. No hay autocompletado en shell
Aunque cobra genera scripts de completado, no hay documentación sobre cómo instalarlos.

---

## 🎨 PROBLEMAS DE UX

### Prioridad: MEDIA

#### 26. IDs largos son difíciles de usar
Los UUIDs completos son difíciles de escribir. Debería haber soporte para IDs cortos (primeros 8 caracteres) o autocompletado.

#### 27. No hay confirmación al cancelar sesiones
El comando `c` en TUI cancela inmediatamente sin confirmación.

#### 28. El progreso visual no tiene color diferenciado
La barra de progreso usa gradiente por defecto pero no cambia de color según el estado (work vs break).

#### 29. No hay sonido configurable
Las notificaciones de sonido son binarias (on/off) sin opción de elegir tono o volumen.

---

## ✅ LO QUE FUNCIONA BIEN

1. **Arquitectura Hexagonal:** Excelente separación de concerns con domain, ports, services y adapters.
2. **Cobertura de tests del dominio:** Los tests de domain y services son completos.
3. **Manejo de configuración:** El sistema de config con Viper funciona bien y crea defaults automáticamente.
4. **Git integration:** La detección de contexto git es robusta y bien testeada.
5. **MCP Server:** Implementación completa con todas las operaciones necesarias.
6. **Storage:** El uso de SQLite con WAL mode y foreign keys es apropiado.
7. **JSON output:** La flag `--json` funciona consistentemente en todos los comandos que la implementan.

---

## 📊 MÉTRICAS DEL PROYECTO

| Aspecto | Estado |
|---------|--------|
| Compilación | ✅ Sin errores |
| Tests | ✅ 100% pasan |
| go vet | ✅ Sin advertencias |
| go fmt | ⚠️ Algunos archivos necesitan formateo |
| Dependencias | ✅ Actualizadas |
| Arquitectura | ✅ Hexagonal/Clean |

---

## 🎯 RECOMENDACIONES PRIORITARIAS

### Inmediatas (Crítico)
1. Corregir el bug de `status` mostrando "Loading..."
2. Formatear todos los archivos con `go fmt`

### Corto plazo (Alta)
3. Agregar comando `delete` para tareas
4. Corregir el uso de `fmt.Scanln` en `stop.go`
5. Implementar graceful shutdown para MCP server
6. Arreglar la inconsistencia de tags `null` vs `[]`

### Mediano plazo (Media)
7. Agregar comando `edit` para tareas
8. Implementar IDs cortos o autocompletado
9. Agregar tests para comandos CLI
10. Mejorar manejo de errores en callbacks TUI

---

## 🔍 NOTAS DE IMPLEMENTACIÓN

### Estructura de archivos recomendada para fixes:

```
cmd/
  delete.go          # Nuevo comando
tui/
  timer.go           # Fix goroutine leak y race condition
mcp/
  mcp_server.go      # Add graceful shutdown
```

### Cambios mínimos sugeridos:

1. **Fix status:** Modificar `ShowStatus` para no usar TUI cuando no hay sesión activa.
2. **Fix tags:** En `task_repository.go`, inicializar siempre el slice.
3. **Add delete command:** Implementar comando CLI que use `taskService.DeleteTask()`.

---

*Reporte generado por análisis automatizado y revisión manual de código.*
