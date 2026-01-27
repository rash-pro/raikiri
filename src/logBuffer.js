class LogBuffer {
    constructor(limit = 50) {
        this.limit = limit;
        this.logs = [];
    }

    add(log) {
        this.logs.push(log);
        if (this.logs.length > this.limit) {
            this.logs.shift();
        }
    }

    replay(socket) {
        this.logs.forEach(log => {
            socket.emit('debug_log', log);
        });
    }
}

module.exports = new LogBuffer();
