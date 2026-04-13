import winston from 'winston';

const { combine, timestamp, printf, colorize } = winston.format;

const customFormat = printf(({ level, message, timestamp, service }) => {
  return `${timestamp} [${service || 'App'}] ${level}: ${message}`;
});

export const logger = winston.createLogger({
  level: 'info',
  format: combine(
    timestamp({ format: 'YYYY-MM-DD HH:mm:ss' }),
    winston.format.errors({ stack: true }),
    winston.format.splat(),
    winston.format.json()
  ),
  defaultMeta: { service: 'Raikiri' },
  transports: [
    new winston.transports.File({ filename: 'data/error.log', level: 'error' }),
    new winston.transports.File({ filename: 'data/combined.log' }),
  ],
});

if (process.env.NODE_ENV !== 'production') {
  logger.add(new winston.transports.Console({
    format: combine(
      colorize(),
      customFormat
    ),
  }));
}

export function createLogger(serviceName: string) {
  return logger.child({ service: serviceName });
}
