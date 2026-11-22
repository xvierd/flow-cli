package i18n

// esIntegrations holds the Spanish strings of `flow integrate` and the
// built-in integrations. It is kept apart from es.go and merged into
// esCatalog at init, so the same printf-verb drift test covers it.
//
//nolint:misspell // Spanish catalog — the misspell linter only knows English.
var esIntegrations = map[string]string{
	// -- flow integrate ------------------------------------------------------
	"Integrations": "Integraciones",
	"enabled":      "activada",
	"disabled":     "desactivada",
	"ready":        "lista",
	"needs setup":  "falta configurar",
	"unsupported":  "no disponible",
	"Enable one with: flow integrate enable <name>  (add --dry-run to preview)": "Actívala con: flow integrate enable <nombre>  (añade --dry-run para previsualizar)",
	"unknown integration %q (available: %s)":                                    "integración desconocida %q (disponibles: %s)",
	"%s cannot be enabled here: %s":                                             "%s no se puede activar aquí: %s",
	"Enabling %s:":                                                              "Activando %s:",
	"Disabling %s:":                                                             "Desactivando %s:",
	"Would enable %s:":                                                          "Se activaría %s:",
	"Would disable %s:":                                                         "Se desactivaría %s:",
	"write %s":                                                                  "escribir %s",
	"remove %s":                                                                 "eliminar %s",
	"add the flow block to %s":                                                  "añadir el bloque de flow a %s",
	"remove the flow block from %s":                                             "quitar el bloque de flow de %s",
	"set [integrations] %s = %v in %s":                                          "poner [integrations] %s = %v en %s",
	"undo what it changed for the current session, if anything": "deshacer lo que haya cambiado en la sesión actual, si es el caso",
	"Dry run: nothing was changed.":                             "Simulación: no se cambió nada.",
	"Change %s? [y/N]":                                          "¿Modificar %s? [s/N]",
	"Nothing was changed. Re-run with --yes to apply it without asking, or make the change yourself.": "No se cambió nada. Vuelve a ejecutarlo con --yes para aplicarlo sin preguntar, o haz el cambio tú.",
	"failed to update %s": "no se pudo actualizar %s",
	"✓ %s enabled.":       "✓ %s activada.",
	"✓ %s disabled.":      "✓ %s desactivada.",
	"  Not ready yet: %s": "  Aún no está lista: %s",

	// -- Descriptions --------------------------------------------------------
	"Do Not Disturb / Focus during work sessions":                "No molestar / Concentración durante las sesiones de trabajo",
	"Slack status \"Focusing until HH:MM\" during work sessions": "Estado de Slack \"En concentración hasta HH:MM\" durante las sesiones de trabajo",
	"Pause Spotify / Music during work sessions (macOS)":         "Pausar Spotify / Música durante las sesiones de trabajo (macOS)",
	"Timer in the tmux status bar (flow tmux-status)":            "Temporizador en la barra de estado de tmux (flow tmux-status)",
	"SketchyBar plugin reading flow status --json":               "Plugin de SketchyBar que lee flow status --json",
	"Waybar custom module reading flow status --json":            "Módulo personalizado de Waybar que lee flow status --json",
	"xbar / SwiftBar menu-bar plugin reading flow status --json": "Plugin de barra de menús para xbar / SwiftBar que lee flow status --json",
	"Focusing until %s": "En concentración hasta las %s",

	// -- Status details ------------------------------------------------------
	"via Shortcuts":                "con Atajos",
	"via dunstctl":                 "con dunstctl",
	"via gsettings (GNOME)":        "con gsettings (GNOME)",
	"missing Shortcuts: %s":        "faltan los atajos: %s",
	"could not list Shortcuts: %v": "no se pudieron listar los atajos: %v",
	"the shortcuts command is not available (macOS 12 or later is required)":         "el comando shortcuts no está disponible (requiere macOS 12 o posterior)",
	"no supported notification daemon found (needs dunstctl or gsettings)":           "no se encontró un gestor de notificaciones compatible (requiere dunstctl o gsettings)",
	"no token: set %s or store it in the keychain (see flow integrate enable slack)": "sin token: define %s o guárdalo en el llavero (ver flow integrate enable slack)",
	"token found":             "token encontrado",
	"only available on macOS": "solo disponible en macOS",
	"osascript not found":     "no se encontró osascript",
	"Spotify, Music (macOS may ask once to allow automation)": "Spotify, Música (macOS puede pedir una vez permiso de automatización)",
	"tmux not found in PATH":                                  "tmux no está en el PATH",
	"not available on %s":                                     "no disponible en %s",
	"plugin not installed (%s)":                               "plugin no instalado (%s)",
	"plugin installed, but %s is not in PATH":                 "plugin instalado, pero %s no está en el PATH",

	// -- Setup notes ---------------------------------------------------------
	"macOS has no command to toggle Focus, so flow runs two Shortcuts. Create them once in the Shortcuts app:":                                                                         "macOS no tiene un comando para activar Concentración, así que flow ejecuta dos atajos. Créalos una vez en la app Atajos:",
	"  1. New shortcut named \"%s\" → add the action \"Set Focus\" → \"Turn Do Not Disturb On\" (until turned off).":                                                                   "  1. Nuevo atajo llamado \"%s\" → añade la acción de Concentración (\"Set Focus\") → activar \"No molestar\" (hasta que se desactive).",
	"  2. New shortcut named \"%s\" → add \"Set Focus\" → \"Turn Do Not Disturb Off\".":                                                                                                "  2. Nuevo atajo llamado \"%s\" → añade la misma acción → desactivar \"No molestar\".",
	"  Open the Shortcuts app with: open -a Shortcuts. Check with: flow integrate list":                                                                                                "  Abre la app Atajos con: open -a Shortcuts. Compruébalo con: flow integrate list",
	"Other shortcut names can be set in config.toml: [integrations] dnd_on_shortcut / dnd_off_shortcut.":                                                                               "Puedes usar otros nombres en config.toml: [integrations] dnd_on_shortcut / dnd_off_shortcut.",
	"Slack needs a user token (xoxp-…) with the users.profile:write scope: create a Slack app at https://api.slack.com/apps, add that user scope and install it to your workspace.":    "Slack necesita un token de usuario (xoxp-…) con el permiso users.profile:write: crea una app en https://api.slack.com/apps, añade ese permiso de usuario e instálala en tu espacio de trabajo.",
	"Then give flow the token in one of these ways (it is never written to config.toml):":                                                                                              "Luego dale el token a flow de una de estas formas (nunca se escribe en config.toml):",
	"  security add-generic-password -a \"$USER\" -s %s -w   # prompts for the token":                                                                                                  "  security add-generic-password -a \"$USER\" -s %s -w   # pide el token",
	"  export %s=xoxp-…   # in your shell profile":                                                                                                                                     "  export %s=xoxp-…   # en el perfil de tu shell",
	"The first time a player is paused, macOS asks to let your terminal (or flow) control it: allow it in that prompt, or later in System Settings → Privacy & Security → Automation.": "La primera vez que se pausa un reproductor, macOS pide permiso para que tu terminal (o flow) lo controle: acéptalo ahí o más tarde en Ajustes del Sistema → Privacidad y seguridad → Automatización.",
	"Reload tmux to see it: tmux source-file %s":                                                                                                                                       "Recarga tmux para verlo: tmux source-file %s",
	"Prefer to edit it yourself? Add this to %s:":                                                                                                                                      "¿Prefieres editarlo tú? Añade esto a %s:",
	"Restart the tmux server (tmux kill-server) or reset status-right for the change to take effect.":                                                                                  "Reinicia el servidor de tmux (tmux kill-server) o restablece status-right para que el cambio surta efecto.",
	"Add the item to your sketchybarrc, then run: sketchybar --reload":                                                                                                                 "Añade el elemento a tu sketchybarrc y ejecuta: sketchybar --reload",
	"Add the module to your Waybar config (and \"custom/flow\" to modules-left/right), then reload Waybar:":                                                                            "Añade el módulo a tu config de Waybar (y \"custom/flow\" a modules-left/right) y recarga Waybar:",
	"xbar picks the plugin up from its plugins folder (refresh from the xbar menu).":                                                                                                   "xbar carga el plugin desde su carpeta de plugins (actualiza desde el menú de xbar).",
	"Using SwiftBar instead? Copy or link %s into your SwiftBar plugin folder.":                                                                                                        "¿Usas SwiftBar? Copia o enlaza %s en tu carpeta de plugins de SwiftBar.",
}

func init() {
	for k, v := range esIntegrations {
		esCatalog[k] = v
	}
}
