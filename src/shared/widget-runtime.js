export function widgetParams() {
    return new URLSearchParams(window.location.search);
}

export function appearanceWithOverrides(appearance = {}) {
    const params = widgetParams();
    return {
        theme: params.get('theme') || appearance.theme || 'glass',
        accentColor: params.get('accent') || appearance.accentColor || '#10b981',
        fontFamily: params.get('font') || appearance.fontFamily || 'Inter, system-ui, sans-serif',
        backgroundOpacity: numberParam(params, 'opacity', appearance.backgroundOpacity ?? 78),
        borderRadius: numberParam(params, 'radius', appearance.borderRadius ?? 8),
        width: numberParam(params, 'width', appearance.width ?? 520),
        showIcons: boolParam(params, 'icons', appearance.showIcons !== false)
    };
}

export function applyWidgetAppearance(appearance = {}, target = document.documentElement) {
    const resolved = appearanceWithOverrides(appearance);
    target.dataset.widgetTheme = resolved.theme;
    target.style.setProperty('--widget-accent', resolved.accentColor);
    target.style.setProperty('--widget-font', resolved.fontFamily);
    target.style.setProperty('--widget-width', `${resolved.width}px`);
    target.style.setProperty('--widget-radius', `${resolved.borderRadius}px`);
    target.style.setProperty('--widget-bg-alpha', String(Math.min(100, Math.max(0, resolved.backgroundOpacity)) / 100));
    target.style.setProperty('--widget-icon-display', resolved.showIcons ? 'grid' : 'none');
    return resolved;
}

function numberParam(params, key, fallback) {
    const raw = params.get(key);
    if (raw === null || raw === '') return fallback;
    const value = Number(raw);
    return Number.isFinite(value) ? value : fallback;
}

function boolParam(params, key, fallback) {
    const raw = params.get(key);
    if (raw === null || raw === '') return fallback;
    return raw === '1' || raw === 'true' || raw === 'yes' || raw === 'on';
}
