const { SNSClient, PublishCommand } = require('@aws-sdk/client-sns');
const { EVENTS } = require('../core/constants.js');
const { getLogger } = require('../logger.js');

const logger = getLogger();

/**
 * Publishes an event to an SNS topic.
 * @param {object} param0 Function parameters
 * @param {string} param0.topicArn The ARN of the SNS topic to publish to
 * @param {object} param0.message The message to publish
 * @returns {Promise<void>}
 */
const publishEvent = async ({ topicArn, message }) => {
  try {
    const client = new SNSClient({
      region: process.env.AWS_REGION,
    });

    const command = new PublishCommand({
      TopicArn: topicArn,
      Message: JSON.stringify(message),
    });

    await client.send(command);
  } catch (error) {
    logger.error(error);
    throw error;
  }
};

/**
 * Publishes a delete media event to the media management topic
 * @param {string} mediaId The ID of the media to delete
 * @returns {Promise<void>}
 */
const publishDeleteMediaEvent = async (mediaId) => {
  const message = {
    type: EVENTS.DELETE_MEDIA.type,
    payload: { mediaId },
  };

  await publishEvent({
    topicArn: EVENTS.DELETE_MEDIA.topicArn,
    message,
  });
};

/**
 * Publishes a delete media event to the media management topic
 * @param {object} param0 Function parameters
 * @param {string} param0.mediaId The ID of the media to delete
 * @param {number} param0.width The width to resize the original image to
 * @returns {Promise<void>}
 */
const publishResizeMediaEvent = async ({ mediaId, width }) => {
  const message = {
    type: EVENTS.RESIZE_MEDIA.type,
    payload: { mediaId, width },
  };

  logger.info({ eventType: message.type, mediaId }, 'Publishing resize event to SNS');

  await publishEvent({
    topicArn: EVENTS.RESIZE_MEDIA.topicArn,
    message,
  });
};

module.exports = {
  publishEvent,
  publishDeleteMediaEvent,
  publishResizeMediaEvent,
};
