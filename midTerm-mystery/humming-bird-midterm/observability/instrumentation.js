const { NodeSDK } = require('@opentelemetry/sdk-node');
const { Resource } = require('@opentelemetry/resources');
const { getNodeAutoInstrumentations } = require('@opentelemetry/auto-instrumentations-node');
const { PeriodicExportingMetricReader } = require('@opentelemetry/sdk-metrics');
const { OTLPTraceExporter } = require('@opentelemetry/exporter-trace-otlp-proto');
const { OTLPMetricExporter } = require('@opentelemetry/exporter-metrics-otlp-proto');
const { ATTR_SERVICE_NAME } = require('@opentelemetry/semantic-conventions');
const { ATTR_DEPLOYMENT_ENVIRONMENT_NAME } = require('@opentelemetry/semantic-conventions/incubating');
const { awsEcsDetector } = require('@opentelemetry/resource-detector-aws');

const init = () => {
  const sdk = new NodeSDK({
    resource: new Resource({
      [ATTR_SERVICE_NAME]: 'hummingbird',
      [ATTR_DEPLOYMENT_ENVIRONMENT_NAME]: process.env.NODE_ENV,
    }),
    resourceDetectors: [awsEcsDetector],
    traceExporter: new OTLPTraceExporter(),
    metricReader: new PeriodicExportingMetricReader({
      exporter: new OTLPMetricExporter(),
    }),
    instrumentations: [
      getNodeAutoInstrumentations({
        '@opentelemetry/instrumentation-dns': {
          enabled: false,
        },
        '@opentelemetry/instrumentation-net': {
          enabled: false,
        },
      }),
    ],
  });

  sdk.start();
  console.log('OpenTelemetry SDK started');

  process.on('SIGTERM', () => {
    sdk
      .shutdown()
      .then(() => process.exit(0))
      .catch((err) => {
        console.error('Error shutting down OpenTelemetry SDK', err);
        process.exit(1);
      });
  });
};

module.exports = { init }
