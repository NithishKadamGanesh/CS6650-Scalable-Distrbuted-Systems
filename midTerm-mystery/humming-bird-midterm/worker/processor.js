const { SQSClient, ReceiveMessageCommand, DeleteMessageCommand } = require('@aws-sdk/client-sqs');
const { S3Client, CopyObjectCommand } = require('@aws-sdk/client-s3');
const { getLogger } = require('../logger.js');
const { getMedia, setMediaStatus, deleteMedia } = require('../clients/dynamodb.js');
const { deleteMediaFile } = require('../clients/s3.js');
const { MEDIA_STATUS } = require('../core/constants.js');

const logger = getLogger();

function requireEnv(name) {
  const v = process.env[name];
  if (!v) throw new Error(`Missing required env var: ${name}`);
  return v;
}

function parseBody(body) {
  // When SNS delivers to SQS (raw disabled), body is an SNS envelope with Message as a JSON string.
  // When raw delivery is enabled, body is the published message JSON.
  try {
    const parsed = JSON.parse(body);
    if (parsed && typeof parsed === 'object' && parsed.Message) {
      return JSON.parse(parsed.Message);
    }
    return parsed;
  } catch {
    return null;
  }
}

async function handleResize({ mediaId }) {
  const bucket = requireEnv('MEDIA_BUCKET_NAME');
  const region = requireEnv('AWS_REGION');

  const media = await getMedia(mediaId);
  if (!media) {
    logger.warn({ mediaId }, 'Resize event received for unknown mediaId');
    return;
  }

  await setMediaStatus({ mediaId, newStatus: MEDIA_STATUS.PROCESSING });

  // Minimal "processor" implementation: copy original -> resized (no actual resize).
  // This unblocks /download and gives you COMPLETE semantics.
  const s3 = new S3Client({ region });
  const sourceKey = `uploads/${mediaId}/${media.name}`;
  const destKey = `resized/${mediaId}/${media.name}`;

  await s3.send(
    new CopyObjectCommand({
      Bucket: bucket,
      CopySource: `${bucket}/${sourceKey}`,
      Key: destKey,
    })
  );

  await setMediaStatus({ mediaId, newStatus: MEDIA_STATUS.COMPLETE });
  logger.info({ mediaId }, 'Resize completed (copy original to resized)');
}

async function handleDelete({ mediaId }) {
  const media = await getMedia(mediaId);
  if (!media) {
    logger.warn({ mediaId }, 'Delete event received for unknown mediaId');
    return;
  }

  // Delete both original and resized objects (best-effort)
  await deleteMediaFile({ mediaId, mediaName: media.name, keyPrefix: 'uploads' });
  await deleteMediaFile({ mediaId, mediaName: media.name, keyPrefix: 'resized' });
  await deleteMedia(mediaId);

  logger.info({ mediaId }, 'Deleted media files + metadata');
}

async function handleMessage(message) {
  const parsed = parseBody(message.Body);
  if (!parsed || !parsed.type || !parsed.payload) {
    logger.warn({ body: message.Body }, 'Skipping unrecognized message body');
    return;
  }

  if (parsed.type === 'media.v1.resize') {
    await handleResize(parsed.payload);
    return;
  }

  if (parsed.type === 'media.v1.delete') {
    await handleDelete(parsed.payload);
    return;
  }

  logger.warn({ type: parsed.type }, 'Skipping unknown event type');
}

async function pollForever() {
  const region = requireEnv('AWS_REGION');
  const queueUrl = requireEnv('MEDIA_QUEUE_URL');

  const sqs = new SQSClient({ region });
  logger.info({ queueUrl }, 'Worker started: polling SQS');

  // eslint-disable-next-line no-constant-condition
  while (true) {
    const resp = await sqs.send(
      new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 10,
        WaitTimeSeconds: 20,
        VisibilityTimeout: 60,
      })
    );

    const messages = resp.Messages || [];
    for (const m of messages) {
      try {
        await handleMessage(m);
        await sqs.send(
          new DeleteMessageCommand({
            QueueUrl: queueUrl,
            ReceiptHandle: m.ReceiptHandle,
          })
        );
      } catch (err) {
        logger.error({ err, messageId: m.MessageId }, 'Failed to process message');
        // leave it in the queue for retry (visibility timeout will expire)
      }
    }
  }
}

pollForever().catch((err) => {
  logger.error({ err }, 'Worker fatal error');
  process.exit(1);
});

