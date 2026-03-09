const { Router } = require('express');
const {
  deleteController,
  downloadController,
  getController,
  resizeController,
  statusController,
  uploadController,
} = require('../controllers/media.js');
const setMediaWidth = require('../middlewares/setMediaWidth.js');
const validateWidth = require('../middlewares/validateWidth.js');

const router = Router();

router.post('/upload', validateWidth, setMediaWidth, uploadController);

router.get('/:id/status', statusController);

router.get('/:id/download', downloadController);

router.get('/:id', getController);

router.put('/:id/resize', validateWidth, setMediaWidth, resizeController);

router.delete('/:id', deleteController);

module.exports = router;
