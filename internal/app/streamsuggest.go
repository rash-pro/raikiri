package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Stream title/tag ideas come from a coding-agent CLI running headless on this machine
// (Claude Code or Codex, picked in the dashboard), so they use the streamer's own login
// and need no API key.

const (
	suggestAgentClaude = "claude"
	suggestAgentCodex  = "codex"
	suggestTimeout     = 2 * time.Minute
)

// Overridable in tests. Returns the raw JSON object matching schema.
var runSuggestAgent = func(ctx context.Context, agent, model, prompt, schema string) ([]byte, error) {
	if agent == suggestAgentCodex {
		return runCodex(ctx, model, prompt, schema)
	}
	return runClaude(ctx, model, prompt, schema)
}

func runClaude(ctx context.Context, model, prompt, schema string) ([]byte, error) {
	bin, err := agentBinary("claude")
	if err != nil {
		return nil, err
	}
	args := []string{"-p",
		"--effort", "low",
		"--tools", "",
		"--no-session-persistence",
		"--setting-sources", "",
		"--strict-mcp-config",
		"--output-format", "json",
		"--json-schema", schema,
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	out, err := runAgentCommand(ctx, bin, args, prompt)
	if err != nil {
		return nil, err
	}
	var result struct {
		IsError          bool            `json:"is_error"`
		Result           string          `json:"result"`
		StructuredOutput json.RawMessage `json:"structured_output"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("respuesta inválida de claude: %w", err)
	}
	if result.IsError || len(result.StructuredOutput) == 0 {
		return nil, fmt.Errorf("claude: %s", result.Result)
	}
	return result.StructuredOutput, nil
}

func runCodex(ctx context.Context, model, prompt, schema string) ([]byte, error) {
	bin, err := agentBinary("codex")
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "raikiri-codex-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	schemaPath, outPath := filepath.Join(dir, "schema.json"), filepath.Join(dir, "out.json")
	if err := os.WriteFile(schemaPath, []byte(schema), 0o600); err != nil {
		return nil, err
	}
	args := []string{"exec",
		"--sandbox", "read-only",
		"--skip-git-repo-check",
		"--ephemeral",
		"--output-schema", schemaPath,
		"--output-last-message", outPath,
		"-c", "model_reasoning_effort=low",
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	// "-" reads the prompt from stdin.
	if _, err := runAgentCommand(ctx, bin, append(args, "-"), prompt); err != nil {
		return nil, err
	}
	return os.ReadFile(outPath)
}

// runAgentCommand passes the prompt on stdin: as an argument its newlines and quotes would
// be mangled when Windows runs an npm-installed CLI through a .cmd shim.
func runAgentCommand(ctx context.Context, bin string, args []string, prompt string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	// Run outside any project so no CLAUDE.md/AGENTS.md leaks into the prompt.
	cmd.Dir = os.TempDir()
	cmd.Stdin = strings.NewReader(prompt)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if lines := strings.Split(msg, "\n"); len(lines) > 3 {
			msg = strings.Join(lines[len(lines)-3:], "\n")
		}
		return nil, fmt.Errorf("%s: %v %s", filepath.Base(bin), err, msg)
	}
	return out, nil
}

// agentBinary finds the CLI even though the systemd user unit may have a minimal PATH
// (both CLIs are commonly installed through mise).
func agentBinary(name string) (string, error) {
	if bin, err := exec.LookPath(name); err == nil {
		return bin, nil
	}
	home, _ := os.UserHomeDir()
	for _, candidate := range []string{
		".local/bin/" + name,
		".local/share/mise/shims/" + name,
		".local/share/mise/installs/" + name + "/latest/" + name,
		".local/share/mise/installs/" + name + "/latest/bin/" + name,
		".claude/local/" + name,
	} {
		path := filepath.Join(home, candidate)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("no se encontró el CLI de %s", name)
}

type streamSuggestion struct {
	Titles []string `json:"titles"`
	Tags   []string `json:"tags"`
}

const streamSuggestSchema = `{
  "type": "object",
  "properties": {
    "titles": {"type": "array", "items": {"type": "string"}, "minItems": 4, "maxItems": 6},
    "tags": {"type": "array", "items": {"type": "string"}, "minItems": 5, "maxItems": 10}
  },
  "required": ["titles", "tags"],
  "additionalProperties": false
}`

type pastBroadcast struct {
	Title    string `json:"title"`
	Date     string `json:"date"`
	Duration string `json:"duration"`
	Views    int    `json:"views"`
}

func (a *App) twitchPastBroadcasts(ctx context.Context, id twitchIdentity) []pastBroadcast {
	var payload struct {
		Data []struct {
			Title     string `json:"title"`
			CreatedAt string `json:"created_at"`
			Duration  string `json:"duration"`
			ViewCount int    `json:"view_count"`
		} `json:"data"`
	}
	q := url.Values{"user_id": {id.userID}, "type": {"archive"}, "first": {"10"}}
	if err := a.twitchHelix(ctx, id, http.MethodGet, "/videos", q, nil, &payload); err != nil {
		a.logger.Warn("stream suggest: could not read past broadcasts", "error", err)
		return nil
	}
	past := make([]pastBroadcast, 0, len(payload.Data))
	for _, v := range payload.Data {
		past = append(past, pastBroadcast{Title: v.Title, Date: strings.SplitN(v.CreatedAt, "T", 2)[0], Duration: v.Duration, Views: v.ViewCount})
	}
	return past
}

// steamProgress reports achievement progress for the stream's game, matched by name.
func (a *App) steamProgress(game *StreamGame) string {
	if game == nil {
		return ""
	}
	apps, err := a.achievementsReader().Apps()
	if err != nil {
		return ""
	}
	for _, app := range apps {
		if strings.EqualFold(app.Game, game.Name) {
			return fmt.Sprintf("%d/%d logros de Steam", app.Unlocked, app.Total)
		}
	}
	return ""
}

func (a *App) handleStreamInfoSuggest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Title string      `json:"title"`
		Game  *StreamGame `json:"game"`
		Tags  []string    `json:"tags"`
		Notes string      `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), suggestTimeout)
	defer cancel()

	brief := map[string]any{
		"fecha_hoy":     time.Now().Format("2006-01-02 (Monday)"),
		"titulo_actual": body.Title,
		"tags_actuales": body.Tags,
	}
	if body.Game != nil {
		brief["juego"] = body.Game.Name
	}
	if progress := a.steamProgress(body.Game); progress != "" {
		brief["progreso"] = progress
	}
	if notes := strings.TrimSpace(body.Notes); notes != "" {
		brief["notas_del_streamer_para_hoy"] = notes
	}
	recent := []streamInfoPreset{}
	_ = a.store.WidgetJSON(ctx, streamInfoRecentKey, &recent)
	brief["titulos_usados_recientemente"] = recent
	if id, err := a.twitchIdentity(ctx); err == nil {
		brief["canal_twitch"] = a.config().TwitchChannel
		if past := a.twitchPastBroadcasts(ctx, id); len(past) > 0 {
			brief["streams_pasados_twitch"] = past
		}
	}
	briefJSON, _ := json.MarshalIndent(brief, "", "  ")

	cfg := a.config()
	out, err := runSuggestAgent(ctx, cfg.SuggestAgent, strings.TrimSpace(cfg.SuggestModel), streamSuggestPrompt+"\n\nContexto:\n"+string(briefJSON), streamSuggestSchema)
	var suggestion streamSuggestion
	if err == nil {
		if err = json.Unmarshal(out, &suggestion); err == nil && len(suggestion.Titles) == 0 {
			err = errors.New("el agente no propuso títulos")
		}
	}
	if err != nil {
		a.logger.Warn("stream suggest failed", "agent", cfg.SuggestAgent, "error", err)
		w.WriteHeader(http.StatusBadGateway)
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	suggestion.Tags = cleanTags(suggestion.Tags)
	titles := suggestion.Titles[:0]
	for _, t := range suggestion.Titles {
		if t = strings.Join(strings.Fields(t), " "); t != "" && validateYouTubeTitle(t) == nil {
			titles = append(titles, t)
		}
	}
	suggestion.Titles = titles
	writeJSON(w, suggestion)
}

const streamSuggestPrompt = `Eres el productor de un canal de streaming de videojuegos (Twitch + YouTube en simultáneo). Propón títulos y tags para el stream de hoy que atraigan espectadores nuevos sin engañarlos.

El canal es muy pequeño: casi nadie llega por seguir al streamer, llegan navegando la categoría del juego en Twitch o el feed de YouTube, donde el título compite al lado de canales enormes con miniaturas casi iguales. El título es la principal razón para que alguien elija este stream en vez de otro, así que cada opción tiene que darle a un desconocido un motivo concreto para entrar ahora: una pregunta que quiera ver resuelta, algo en juego, un número que da contexto, personalidad. Un título genérico ("jugando FFVIII", "día 2") es invisible; si una opción no la abrirías tú viéndola entre 30 streams del mismo juego, cámbiala.

Títulos:
- Máximo 100 caracteres (límite de YouTube; el mismo título va a ambas plataformas). Mejor si las primeras ~45 caracteres funcionan solos, porque es lo que se ve en la miniatura de Twitch y en el feed de YouTube.
- Respeta el formato e idioma que el streamer ya usa (prefijos como [ES/EN], emojis característicos, el nombre del reto o serie) para que el canal se reconozca, pero varía el gancho: curiosidad, progreso concreto del día, momento icónico del juego que toca, reto/stakes, interacción con el chat.
- Si hay streams pasados del mismo juego, trata el de hoy como continuación (día/parte N, dónde va la partida) en vez de repetir el título de presentación.
- Incluye el nombre del juego en al menos la mitad de las opciones: YouTube indexa el título y no recibe la categoría del juego.
- No inventes actividades concretas del día (minijuegos, jefes, zonas, dinámicas con el chat) que no estén en las notas del streamer; si no hay notas, apóyate en el reto, el progreso y la continuidad.
- Nada de spoilers de la trama, ni MAYÚSCULAS completas, ni clickbait que el stream no cumpla.
- Da opciones claramente distintas entre sí, ordenadas de la que más clics atraería de desconocidos a la que menos.

Tags (una sola lista, se aplica igual a Twitch y YouTube):
- Hasta 10 tags; cada uno solo letras y números (sin espacios, guiones ni #), máximo 25 caracteres. Usa CamelCase para frases (p. ej. FinalFantasy, PrimeraVez).
- Mezcla: idioma(s) del stream, juego/saga, género, el formato del reto y algo que describa la vibra del canal. Prioriza tags que la gente realmente busca en Twitch sobre tags ingeniosos que nadie usa; para un canal chico rinden más los tags de nicho con poca competencia (idioma + formato del reto) que los genéricos saturados.
- Escribe los tags con su ortografía real, incluidos acentos y ñ (Español, no Espanol).
- Conserva los tags actuales que sigan teniendo sentido.`
