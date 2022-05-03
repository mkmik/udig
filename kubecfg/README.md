This project uses [kubecfg](https://github.com/ksonnet/kubecfg) to describe a parametric kubernetes config.

In order to create your custom deployment create a .jsonnet file with something like this in it:

```
(import "https://raw.githubusercontent.com/mkmik/udig/main/kubecfg/udig.jsonnet") {
  certificate+: {
    domainName:: 'yourdomain.io',
  },
}
```

Then apply it with:

```
kubecfg update udig.jsonnet
```

When you're happy with it we recommend you replace `master` with the current commit SHA of the udig repo.
