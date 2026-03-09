const sendOkResponse = (res, data) => {
  res.status(200).send(data);
};

const sendAcceptedResponse = (res, data) => {
  res.status(202).send(data);
};

const sendNoContentResponse = (res) => {
  res.status(204).send();
};

const sendBadRequestResponse = (res, error) => {
  res.status(400).send({ message: error?.message || 'Bad request' });
};

const sendNotFoundResponse = (res) => {
  res.status(404).send({ message: 'Not found' });
};

const sendResponse = (res, status, message) => {
  res.status(status).send({ message });
};

const sendErrorResponse = (res, error) => {
  res
    .status(error?.status || 500)
    .send(error?.message || 'Internal server error');
};

module.exports = {
  sendOkResponse,
  sendAcceptedResponse,
  sendNoContentResponse,
  sendBadRequestResponse,
  sendNotFoundResponse,
  sendResponse,
  sendErrorResponse,
};
