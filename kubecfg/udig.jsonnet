local kube = import 'kube.libsonnet';

{
  namespace:: { metadata+: { namespace: 'udig' } },
  ns: kube.Namespace($.namespace.metadata.namespace),

  svc: kube.Service('udig') + $.namespace {
    local service = self,
    local container = service.target_pod.spec.containers_[service.target_pod.spec.default_container],

    target_pod: $.udig.spec.template,
    spec+: {
      ports: [
        {
          name: 'uplink',
          port: 4000,
          targetPort: container.ports_.uplink.containerPort,
        },
        {
          name: 'dns',
          port: 53,
          targetPort: container.ports_.dns.containerPort,
        },
        {
          name: 'https',
          port: 443,
          targetPort: container.ports_.https.containerPort,
        },
      ],

      type: 'LoadBalancer',
    },
  },

  udig: kube.Deployment('udig') + $.namespace {
    spec+: {
      template+: {
        spec+: {
          default_container: 'udigd',
          containers_+: {
            debug: kube.Container('debug') {
              image: 'ubuntu',
              args: ['/bin/sleep', '10000000'],
              resources+: {
                requests+: { memory: '10Mi' },
              },
            },

            udigd: kube.Container('udigd') {
              image: 'mkmik/udigd@sha256:fbd1dca7fdbc2feb473f568cf064555073c2eb26064c43ac16cdd1f5ae04b394',
              args: [
                '-logtostderr',
                '-http',
                ':8080',  // debug, metrics

                '-uplink',
                ':4000',

                '-port',
                '53',
                '-port',
                '443',

                '-cert',
                '/certs/tls.crt',
                '-key',
                '/certs/tls.key',
              ],
              securityContext: {
                capabilities: {
                  drop: ['all'],
                  add: ['NET_BIND_SERVICE'],
                },
              },
              ports_+: {
                uplink: { containerPort: 4000 },
                dns: { containerPort: 53 },
                https: { containerPort: 443 },
              },
              volumeMounts_+: {
                certs: {
                  mountPath: '/certs',
                },
              },
              resources+: {
                requests+: { memory: '10Mi' },
              },
            },
          },
          volumes_+: {
            certs: {
              secret: { secretName: 'udig' },
            },
          },
          automountServiceAccountToken: false,
          terminationGracePeriodSeconds: 1,
        },
      },
    },
  },

  certificate: kube._Object('certmanager.k8s.io/v1alpha1', 'Certificate', 'udig') + $.namespace {
    local this = self,
    domainName:: 'udig.io',

    spec: {
      secretName: this.metadata.name,
      issuerRef: {
        name: 'letsencrypt-prod',
        kind: 'ClusterIssuer',
      },
      dnsNames: ['*.%s' % [this.domainName]],
      acme: {
        config: [{
          dns01: { provider: 'clouddns' },
          domains: this.spec.dnsNames,
        }],
      },
    },
  },
}
