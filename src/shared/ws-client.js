export function connectEvents(path, handlers = {}) {
    let socket;
    let retryMs = 500;
    const url = () => `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}${path}`;

    function connect() {
        socket = new WebSocket(url());
        socket.addEventListener('open', () => {
            retryMs = 500;
            handlers.connect?.();
        });
        socket.addEventListener('message', (event) => {
            try {
                const payload = JSON.parse(event.data);
                handlers[payload.event]?.(payload.data);
            } catch (err) {
                console.warn('Invalid WebSocket payload', err);
            }
        });
        socket.addEventListener('close', () => {
            handlers.disconnect?.();
            setTimeout(connect, retryMs);
            retryMs = Math.min(retryMs * 2, 10000);
        });
        socket.addEventListener('error', () => socket.close());
    }

    connect();
    return {
        close() {
            if (socket) socket.close();
        }
    };
}
