const { sendBadRequestResponse } = require('../core/responses.js');
const { MEDIA_WIDTH } = require('../core/constants.js');

const { MAX_MEDIA_WIDTH, MIN_MEDIA_WIDTH } = MEDIA_WIDTH;

/**
 * Validate the media width option from the query string or request body.
 * @param req
 * @param res
 * @param next
 * @returns void
 */
const middleware = (req, res, next) => {
  const { width: widthFromQs } = req.query;
  const { width: widthFromBody } = req.body;

  const width = widthFromQs || widthFromBody;

  if (!validMediaWidth(width)) {
    sendBadRequestResponse(res, {
      message: `width should be a value between ${MIN_MEDIA_WIDTH} and ${MAX_MEDIA_WIDTH}`,
    });
    return;
  }

  next();
};

/**
 * Validates if the width parameter is an integer and falls within the
 * expected values.
 * @param {any} width width parameter from the query string
 * @returns {boolean} whether the given value is valid
 */
const validMediaWidth = (width) => {
  if (!width) {
    return true;
  }

  const intMediaWidth = parseInt(width, 10);

  if (isNaN(intMediaWidth)) {
    return false;
  }

  return intMediaWidth >= MIN_MEDIA_WIDTH && intMediaWidth <= MAX_MEDIA_WIDTH;
};

module.exports =  middleware;
