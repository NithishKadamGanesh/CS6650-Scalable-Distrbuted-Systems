module.exports = {
  MAX_FILE_SIZE: 100 * 1024 * 1024, // 100 MB

  CUSTOM_FORMIDABLE_ERRORS: {
    INVALID_FILE_TYPE: {
      code: 9000,
      httpCode: 400,
    },
  },

  EVENTS: {
    DELETE_MEDIA: {
      topicArn: process.env.MEDIA_MANAGEMENT_TOPIC_ARN,
      type: 'media.v1.delete',
    },
    RESIZE_MEDIA: {
      topicArn: process.env.MEDIA_MANAGEMENT_TOPIC_ARN,
      type: 'media.v1.resize-media',
    },
  },

  MEDIA_STATUS: {
    PENDING: 'PENDING',
    PROCESSING: 'PROCESSING',
    COMPLETE: 'COMPLETE',
    ERROR: 'ERROR',
  },

  MEDIA_WIDTH: {
    DEFAULT_MEDIA_WIDTH: 500,
    MIN_MEDIA_WIDTH: 100,
    MAX_MEDIA_WIDTH: 1024,
  },
};
