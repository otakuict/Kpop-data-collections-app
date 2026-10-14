# GitOps deployment on the private K3s server

The three source workflows remain independent. A successful `main` build pushes a full commit-SHA image to Docker Hub, then sends `image-published` to `otakuict/private-server-devops`. That repository opens one image update PR per service; a human reviews and merges it. Argo CD deploys the selected tag after merge.

Before merging the GitOps workflow change, set these GitHub repository variables:

| Variable | Value |
| --- | --- |
| `DEPLOYMENT_MODE` | `gitops` |
| `DOCKERHUB_USERNAME` | `otakuict` |
| `VITE_BACKOFFICE_URL` | `/backoffice/` |
| `VITE_GALLERY_URL` | `/` |

Keep the existing `DOCKERHUB_TOKEN` repository secret. Add `GITOPS_DISPATCH_TOKEN` as a repository secret, using a fine-grained PAT with Contents: Write limited to `otakuict/private-server-devops`. Do not put any token in Git or chat. The `ADMIN_TOKEN` used by the K3s backend is a separate Kubernetes Secret. A GitHub Actions secret cannot be read back for migration.

The workflows select either the existing Docker deploy job or the GitOps dispatch job by `DEPLOYMENT_MODE`; they never run both for the same build. Set the variable before the next `main` build. Once configured, run Backend, Frontend, and Backoffice workflows on `main` separately. Verify each image is published and a DevOps image PR appears. A source PR only verifies/builds; it does not publish or deploy.

The release Backoffice image supports `/backoffice/` and is built with Vite base `/backoffice/` in GitOps mode. Gallery and Backoffice use relative links, so a public hostname is not needed at build time. The Nginx images use the container's DNS resolver at startup, allowing the same images to proxy the backend Service on Docker or K3s.

The K3s manifests and rollout sequence are in the DevOps repository's `docs/bias-archive.md`. Its initial replica counts are zero. Do not enable the backend until its administrator Secret and SQLite PVC are ready. Route the public hostname to the internal gateway only after `/` and `/backoffice/` work on the server.
