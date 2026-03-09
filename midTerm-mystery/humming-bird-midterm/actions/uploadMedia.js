const { Transform } = require('node:stream');
const { randomUUID } = require('node:crypto');
const { formidable, errors: formidableErrors } = require('formidable');
const opentelemetry = require('@opentelemetry/api');
const { uploadMediaToStorage } = require('../clients/s3.js');
const {
  MAX_FILE_SIZE,
  CUSTOM_FORMIDABLE_ERRORS,
} = require('../core/constants.js');

const tracer = opentelemetry.trace.getTracer('hummingbird-media-upload');
const meter = opentelemetry.metrics.getMeter('hummingbird-media-upload');
const successesCounter = meter.createCounter('media.upload.success', {
  description: 'Count of successful media uploads',
});
const failuresCounter = meter.createCounter('media.upload.failure', {
  description: 'Count of failed media uploads',
});

/**
 * Uploads a media file to AWS S3 in a streaming fashion.
 * @param {Request} req Express.js (Node) HTTP request object.
 * @returns {Promise<string>} The file ID.
 */
const uploadMedia = async (req) => {
  return new Promise((resolve, reject) => {
    try {
      tracer.startActiveSpan('upload-media-file', (span) => {
        const mediaId = randomUUID();
        span.setAttribute('media.id', mediaId);

        const form = formidable({
          maxFiles: 1,
          minFileSize: 1,
          maxFileSize: MAX_FILE_SIZE,
          keepExtensions: true,
          filter: ({ mimetype }) => {
            const isImage = mimetype && mimetype.startsWith('image');
            if (!isImage) {
              const { code, httpCode } =
                CUSTOM_FORMIDABLE_ERRORS.INVALID_FILE_TYPE;
              const error = new formidableErrors.default(
                'invalidFileType',
                code,
                httpCode
              );
              form.emit('error', error);
              return false;
            }

            return true;
          },
        });

        form.parse(req, (error, fields, files) => {
          if (!Object.keys(files).length) {
            const error = new formidableErrors.default(
              'noFilesFound',
              formidableErrors.malformedMultipart,
              400
            );
            form.emit('error', error);
            return;
          }

          if (error) {
            reject(error);
          }
        });

        form.on('fileBegin', (name, file) => {
          /*
           * Override the default file.open and file.end functions.
           * The file is uploaded S3 once it's open with a stream.
           */
          file.open = function () {
            this._writeStream = new Transform({
              transform(chunk, encoding, callback) {
                this.push(chunk);
                callback();
              },
            });

            this._writeStream.on('error', (error) => {
              form.emit('error', error);
            });

            uploadMediaToStorage({
              mediaId,
              mediaName: file.originalFilename,
              body: this._writeStream,
            })
              .then(() => {
                span.setAttribute('file.name', file.originalFilename);
                form.emit('data', { event: 'done', file });
              })
              .catch((error) => {
                form.emit('error', error);
              });
          };

          file.end = function (callback) {
            this._writeStream.on('finish', () => {
              this.emit('end');
              callback();
            });
            this._writeStream.end();
          };
        });

        form.on('error', (error) => {
          failuresCounter.add(1);
          span.setStatus({
            code: opentelemetry.SpanStatusCode.ERROR,
            message: error.message,
          });
          span.end();

          reject(error);
        });

        form.on('data', (data) => {
          if (data.event === 'done') {
            successesCounter.add(1);
            span.setStatus({ code: opentelemetry.SpanStatusCode.OK });
            span.end();

            resolve({ mediaId, file: data.file.toJSON() });
          }
        });
      });
    } catch (error) {
      reject(error);
    }
  });
};

module.exports = { uploadMedia };
