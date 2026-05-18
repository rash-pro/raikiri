import { connectEvents } from "/shared/ws-client.js";
import { applyWidgetAppearance, widgetParams } from "/shared/widget-runtime.js";

const mount = document.getElementById('mount');
const params = widgetParams();
const widgetId = params.get('id') || '';
let activeWidget = null;
let latestState = null;
let frame = null;

function render(state) {
    latestState = state;
    const widgets = state?.config?.custom || [];
    activeWidget = widgets.find(widget => widget.id === widgetId);
    if (!activeWidget || activeWidget.enabled === false) {
        mount.innerHTML = '<div class="empty">Custom widget not found or disabled.</div>';
        frame = null;
        return;
    }
    applyWidgetAppearance(activeWidget.appearance || {});
    if (!frame) {
        frame = document.createElement('iframe');
        frame.className = 'custom-frame';
        frame.setAttribute('sandbox', 'allow-scripts');
        mount.replaceChildren(frame);
    }
    const src = buildDocument(activeWidget);
    if (frame.srcdoc !== src) {
        frame.srcdoc = src;
        frame.addEventListener('load', postState, { once: true });
    } else {
        postState();
    }
}

function buildDocument(widget) {
    const appearance = widget.appearance || {};
    return `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<style>
:root {
  --widget-accent: ${cssValue(appearance.accentColor || '#10b981')};
  --widget-font: ${cssValue(appearance.fontFamily || 'Inter, system-ui, sans-serif')};
  --widget-radius: ${Number(appearance.borderRadius || 8)}px;
  --widget-bg-alpha: ${Math.min(100, Math.max(0, Number(appearance.backgroundOpacity ?? 78))) / 100};
}
html, body { margin: 0; background: transparent; color: #fff; font-family: var(--widget-font); overflow: hidden; }
* { box-sizing: border-box; }
${widget.css || ''}
</style>
</head>
<body>
${widget.html || ''}
<script>
window.raikiriState = null;
window.addEventListener('message', event => {
  if (!event.data || event.data.type !== 'raikiri:state') return;
  window.raikiriState = event.data.state;
  window.dispatchEvent(new CustomEvent('raikiri:state', { detail: event.data.state }));
});
window.addEventListener('message', event => {
  if (!event.data || event.data.type !== 'raikiri:event') return;
  window.dispatchEvent(new CustomEvent('raikiri:event', { detail: event.data.event }));
});
${widget.js || ''}
</script>
</body>
</html>`;
}

function postState() {
    if (!frame || !latestState) return;
    frame.contentWindow?.postMessage({ type: 'raikiri:state', state: latestState }, '*');
}

function dispatchCustomEvent(data) {
    if (!frame || !data?.event || !Array.isArray(data.widgetIds) || !data.widgetIds.includes(widgetId)) return;
    frame.contentWindow?.postMessage({ type: 'raikiri:event', event: data.event }, '*');
}

function cssValue(value) {
    return String(value).replace(/[<>{};]/g, '');
}

fetch('/api/widgets/state')
    .then(res => res.json())
    .then(render)
    .catch(console.error);

connectEvents('/ws/widgets', {
    state: render,
    custom_event: dispatchCustomEvent,
    connect: () => console.log('Connected to Custom widget')
});
