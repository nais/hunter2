# hunter2

hunter2 is a daemon that performs one-way synchronization of secrets from Google Secret Manager to Kubernetes Secrets.

### Installation

```shell script
make install
```

### Configuration

Set up the required environment variables

```text
HUNTER2_GOOGLE_PROJECT_ID=some-id
HUNTER2_GOOGLE_PUBSUB_SUBSCRIPTION_ID=some-subscription-id
HUNTER2_DEBUG=false
HUNTER2_NAMESPACE=default
HUNTER2_KUBECONFIG_PATH=$KUBECONFIG
GOOGLE_APPLICATION_CREDENTIALS=/Users/<user>/.config/gcloud/application_default_credentials.json
```

### Run

```shell script
make local
```