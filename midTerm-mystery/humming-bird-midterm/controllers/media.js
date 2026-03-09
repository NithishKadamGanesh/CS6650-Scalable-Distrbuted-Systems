const { errors: formidableErrors } = require('formidable');
const {
  sendAcceptedResponse,
  sendOkResponse,
  sendErrorResponse,
  sendResponse,
  sendBadRequestResponse,
  sendNotFoundResponse,
} = require('../core/responses.js');
const { uploadMedia } = require('../actions/uploadMedia.js');
const { convertBytesToMb } = require('../core/utils.js');
const {
  MAX_FILE_SIZE,
  CUSTOM_FORMIDABLE_ERRORS,
  MEDIA_STATUS,
} = require('../core/constants.js');
const { copyMediaFile, getProcessedMediaUrl } = require('../clients/s3.js');
const { createMedia, getMedia, setMediaStatus } = require('../clients/dynamodb.js');
const { getLogger } = require('../logger.js');
const {
  publishDeleteMediaEvent,
  publishResizeMediaEvent,
} = require('../clients/sns.js');

const logger = getLogger();

const uploadController = async (req, res) => {
  try {
    const { mediaId, file } = await uploadMedia(req);
    const { size, originalFilename: name, mimetype } = file;
    const { width } = req.hummingbirdOptions;

    await createMedia({ mediaId, size, name, mimetype, width });
    // Kick off async processing (handled by the background worker subscribed to media events)
    await publishResizeMediaEvent({ mediaId, width });

    sendAcceptedResponse(res, { mediaId });
  } catch (error) {
    if (error.httpCode && error.code) {
      if (error.code === formidableErrors.biggerThanTotalMaxFileSize) {
        const maxFileSize = convertBytesToMb(MAX_FILE_SIZE);
        let message = `Failed to upload media. Check the file size. Max size is ${maxFileSize} MB.`;
        sendResponse(res, error.httpCode, message);
        return;
      }

      if (error.code === formidableErrors.maxFilesExceeded) {
        sendBadRequestResponse(res, {
          message:
            'Too many fields in the form. Only single file uploads are supported.',
        });
        return;
      }

      if (error.code === formidableErrors.malformedMultipart) {
        sendBadRequestResponse(res, {
          message: 'Malformed multipart form data.',
        });
        return;
      }

      if (error.code === CUSTOM_FORMIDABLE_ERRORS.INVALID_FILE_TYPE.code) {
        sendResponse(
          res,
          CUSTOM_FORMIDABLE_ERRORS.INVALID_FILE_TYPE.httpCode,
          'Invalid file type. Only images are supported.'
        );
        return;
      }

      sendBadRequestResponse(res);
      return;
    }

    logger.error(error);
    sendErrorResponse(res);
  }
};

const statusController = async (req, res) => {
  try {
    const mediaId = req.params.id;
    const media = await getMedia(mediaId);

    if (!media) {
      sendNotFoundResponse(res);
      return;
    }

    logger.info({ mediaId, status: media.status }, 'Status check');
    sendOkResponse(res, { status: media.status });
  } catch (error) {
    logger.error(error);
    sendErrorResponse(res);
  }
};

const downloadController = async (req, res) => {
  try {
    const mediaId = req.params.id;

    const media = await getMedia(mediaId);
    if (!media) {
      sendNotFoundResponse(res);
      return;
    }

    if (media.status !== MEDIA_STATUS.COMPLETE) {
      const SIXTY_SECONDS = 60;
      res.set('Retry-After', SIXTY_SECONDS);
      res.set('Location', `http://${req.get('host')}/v1/media/${mediaId}/status`);
      logger.info(
        { mediaId, currentStatus: media.status },
        'Media not ready for download, sending 202'
      );
      sendAcceptedResponse(res, {
        message: 'Media processing in progress.',
      });
      return;
    }

    logger.info({ mediaId }, 'Media is COMPLETE, generating presigned URL');
    const url = await getProcessedMediaUrl({ mediaId, mediaName: media.name });

    res.redirect(302, url);
  } catch (error) {
    logger.error(error);
    sendErrorResponse(res);
  }
};

const getController = async (req, res) => {
  try {
    const mediaId = req.params.id;
    const media = await getMedia(mediaId);

    if (!media) {
      sendNotFoundResponse(res);
      return;
    }

    sendOkResponse(res, media);
  } catch (error) {
    logger.error(error);
    sendErrorResponse(res);
  }
};

const resizeController = async (req, res) => {
  try {
    const mediaId = req.params.id;

    const media = await getMedia(mediaId);
    if (!media) {
      sendNotFoundResponse(res);
      return;
    }

    const { width } = req.hummingbirdOptions;

    // Learner-lab friendly: perform the work synchronously so users can get COMPLETE without a separate processor.
    await setMediaStatus({ mediaId, newStatus: MEDIA_STATUS.PROCESSING });
    await copyMediaFile({ mediaId, mediaName: media.name });
    await setMediaStatus({ mediaId, newStatus: MEDIA_STATUS.COMPLETE });

    // Still publish the event for environments where a real resizer exists.
    await publishResizeMediaEvent({ mediaId, width });

    sendOkResponse(res, { mediaId, status: MEDIA_STATUS.COMPLETE });
  } catch (error) {
    logger.error(error);
    sendErrorResponse(res);
  }
};

const deleteController = async (req, res) => {
  try {
    const mediaId = req.params.id;

    const media = await getMedia(mediaId);
    if (!media) {
      sendNotFoundResponse(res);
      return;
    }

    await publishDeleteMediaEvent(mediaId);

    sendAcceptedResponse(res, { mediaId });
  } catch (error) {
    logger.error(error);
    sendErrorResponse(res);
  }
};

module.exports = {
  uploadController,
  statusController,
  downloadController,
  getController,
  resizeController,
  deleteController,
};
