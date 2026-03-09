const winston = require('winston');

const { combine, json, timestamp } = winston.format;

/**
 * Logger instance.
 * @type {winston.Logger}
 */
let logger;

/**
 * Initialize the logger.
 * @returns {void}
 */
const init = () => {
  logger = winston.createLogger({
    level: 'info',
    format: combine(timestamp(), json()),
    defaultMeta: { service: 'hummingbird' },
    transports: [new winston.transports.Console()],
  });
};

/**
 * Get the logger instance.
 * @returns {winston.Logger}
 */
const getLogger = () => {
  if (!logger) {
    init();
  }

  return logger;
};

module.exports = { init, getLogger };
