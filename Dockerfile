FROM oven/bun:slim

WORKDIR /app

# Copy package.json and lockfile (if exists)
COPY package.json ./

# Install dependencies via bun
RUN bun install --production

# Copy source code
COPY . .

# Ensure data directory exists
RUN mkdir -p data

# Expose server port
EXPOSE 30001

# Start the application
CMD ["bun", "run", "src/core/server.ts"]
