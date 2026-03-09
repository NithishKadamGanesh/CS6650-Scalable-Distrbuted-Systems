const { MEDIA_WIDTH } = require('../core/constants.js');

const { DEFAULT_MEDIA_WIDTH } = MEDIA_WIDTH;

/**
 * Extract additional configuration options from the request query string.
 * @param req
 * @param res
 * @param next
 * @returns void
 */
const middleware = (req, res, next) => {
  const { width: widthFromQs } = req.query;
  const { width: widthFromBody } = req.body;

  const width = widthFromQs || widthFromBody;

  req.hummingbirdOptions = {
    ...req?.hummingbirdOptions,
    width: width ? parseInt(width) : DEFAULT_MEDIA_WIDTH,
  };

  next();
};

module.exports =  middleware;
