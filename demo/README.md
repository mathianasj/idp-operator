## Install

1. `oc create -f ./deploy/base-app.yaml`
1.
  ```
  kubectl create secret generic controller-manager \
    -n mathianasj-backstage-runner \
    --from-literal=github_token=REPLACE_YOUR_TOKEN_HERE
  ```