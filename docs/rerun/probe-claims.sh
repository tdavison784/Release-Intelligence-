#!/bin/zsh
# Live re-probe of Phase 1 research claims marked UNVERIFIED / blocked / unreachable.
# Output TSV: product<TAB>claim (research doc)<TAB>URL<TAB>http<TAB>redirects<TAB>bytes<TAB>note
TOK=$(gh auth token)
p(){ # product claim url [extra curl args...]
  local prod=$1 claim=$2 url=$3; shift 3
  local res
  res=$(curl -sL -o /dev/null -m 40 -w "%{http_code}\t%{num_redirects}\t%{size_download}" "$@" "$url" 2>/dev/null) || res="000\t0\t0"
  printf '%s\t%s\t%s\t%b\n' "$prod" "$claim" "$url" "$res"; sleep 0.4
}
GH=(-H "Authorization: Bearer $TOK" -H "Accept: application/vnd.github+json")
OCI=(-H "Accept: application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json")
for r in argoproj/argo-cd cert-manager/cert-manager crossplane/crossplane external-secrets/external-secrets kubernetes/ingress-nginx istio/istio postgres/postgres hashicorp/vault redis/redis falcosecurity/falco; do
  p ${r#*/} "api.github.com releases API blocked" "https://api.github.com/repos/$r/releases?per_page=1" $GH
  p ${r#*/} "GHSA advisories list blocked" "https://api.github.com/repos/$r/security-advisories?per_page=1" $GH
  p ${r#*/} "github.com HTML releases blocked" "https://github.com/$r/releases"
  p ${r#*/} "github.com HTML advisories blocked" "https://github.com/$r/security/advisories"
done
p cert-manager "quay.io tags list (UNVERIFIED)" "https://quay.io/api/v1/repository/jetstack/cert-manager-controller/tag/?limit=1&onlyActiveTags=true"
p cert-manager "quay.io manifest v1.18.0" "https://quay.io/v2/jetstack/cert-manager-controller/manifests/v1.18.0" $OCI
p cert-manager "charts.jetstack.io index" "https://charts.jetstack.io/index.yaml"
p cert-manager "artifacthub page" "https://artifacthub.io/packages/helm/cert-manager/cert-manager"
p cert-manager "cert-manager.io docs" "https://cert-manager.io/docs/releases/release-notes/release-notes-1.18/"
p cert-manager "cert-manager.io docs (README pattern)" "https://cert-manager.io/docs/release-notes/"
p cert-manager "osv.dev" "https://api.osv.dev/v1/vulns/GHSA-hcg3-q754-cr77"
p cert-manager "vuln.go.dev" "https://vuln.go.dev/index/db.json.gz"
p cert-manager "gcs release bucket (private 403)" "https://storage.googleapis.com/cert-manager-release/"
p cert-manager "quay.io startupapicheck v1.18.0" "https://quay.io/v2/jetstack/cert-manager-startupapicheck/manifests/v1.18.0" $OCI
p cert-manager "release asset checksum 404 claim" "https://github.com/cert-manager/cert-manager/releases/download/v1.18.0/cert-manager.yaml.sha256"
p argo-cd "quay.io argocd manifest" "https://quay.io/v2/argoproj/argocd/manifests/v3.5.3" $OCI
p argo-cd "argoproj.github.io helm index" "https://argoproj.github.io/argo-helm/index.yaml"
p argo-cd "releases.atom" "https://github.com/argoproj/argo-cd/releases.atom"
p argo-cd "tags.atom" "https://github.com/argoproj/argo-cd/tags.atom"
p argo-cd "release v3.4.0 object" "https://api.github.com/repos/argoproj/argo-cd/releases/tags/v3.4.0" $GH
p argo-cd "release v3.4.1 object" "https://api.github.com/repos/argoproj/argo-cd/releases/tags/v3.4.1" $GH
p argo-cd "artifacthub" "https://artifacthub.io/api/v1/packages/helm/argo/argo-cd"
p external-secrets "charts.external-secrets.io" "https://charts.external-secrets.io/index.yaml"
p external-secrets "external-secrets.github.io" "https://external-secrets.github.io/external-secrets/index.yaml"
p external-secrets "external-secrets.io docs" "https://external-secrets.io/latest/"
p external-secrets "osv" "https://api.osv.dev/v1/query" -X POST -d '{"package":{"name":"github.com/external-secrets/external-secrets","ecosystem":"Go"}}'
for h in charts.crossplane.io/stable/index.yaml releases.crossplane.io/ cli.crossplane.io/ docs.crossplane.io/ crossplane.github.io/ xpkg.crossplane.io/v2/ xpkg.upbound.io/v2/; do p crossplane "blocked host $h" "https://$h"; done
p crossplane "xpkg.crossplane.io manifest" "https://xpkg.crossplane.io/v2/crossplane/crossplane/manifests/v2.0.0" $OCI
p crossplane "s3 helm charts bucket" "https://crossplane-helm-charts.s3.amazonaws.com/"
p crossplane "artifacthub" "https://artifacthub.io/api/v1/packages/helm/crossplane/crossplane"
p ingress-nginx "registry.k8s.io" "https://registry.k8s.io/v2/ingress-nginx/controller/manifests/v1.12.0" $OCI
p ingress-nginx "k8s.gcr.io" "https://k8s.gcr.io/v2/ingress-nginx/controller/manifests/v1.9.0" $OCI
p ingress-nginx "kubernetes.github.io helm" "https://kubernetes.github.io/ingress-nginx/index.yaml"
p ingress-nginx "kubernetes.io blog retirement" "https://kubernetes.io/blog/2025/11/11/ingress-nginx-retirement/"
p ingress-nginx "k8s official CVE feed" "https://kubernetes.io/docs/reference/issues-security/official-cve-feed/index.json"
p ingress-nginx "osv" "https://api.osv.dev/v1/vulns/GHSA-hcg3-q754-cr77"
p ingress-nginx "gcr staging 401" "https://gcr.io/v2/k8s-staging-ingress-nginx/controller/tags/list"
p istio "blob.istio.io helm index" "https://blob.istio.io/istio-release/charts/index.yaml"
p istio "blob.istio.io releases dir" "https://blob.istio.io/istio-release/releases/1.31.1/"
p istio "registry.istio.io" "https://registry.istio.io/v2/"
p istio "gcr.io istio-release tags" "https://gcr.io/v2/istio-release/pilot/tags/list"
p istio "quay.io" "https://quay.io/v2/"
p istio "artifacthub" "https://artifacthub.io/api/v1/packages/helm/istio/istiod"
p istio "github release istio 1.31.1" "https://api.github.com/repos/istio/istio/releases/tags/1.31.1" $GH
p postgresql "ftp.postgresql.org tarball" "https://ftp.postgresql.org/pub/source/v17.2/postgresql-17.2.tar.bz2" -r 0-100
p postgresql "www.postgresql.org security" "https://www.postgresql.org/support/security/"
p postgresql "www.postgresql.org versioning" "https://www.postgresql.org/support/versioning/"
p postgresql "www.postgresql.org release 17.2" "https://www.postgresql.org/docs/release/17.2/"
p postgresql "git.postgresql.org" "https://git.postgresql.org/gitweb/?p=postgresql.git;a=summary"
p postgresql "github archive tarball" "https://github.com/postgres/postgres/archive/refs/tags/REL_17_2.tar.gz" -r 0-100
p postgresql "codeload tarball" "https://codeload.github.com/postgres/postgres/tar.gz/refs/tags/REL_17_2" -r 0-100
p postgresql "apt.postgresql.org" "https://apt.postgresql.org/pub/repos/apt/dists/"
p postgresql "yum.postgresql.org" "https://yum.postgresql.org/"
p postgresql "docker hub library/postgres" "https://hub.docker.com/v2/repositories/library/postgres/tags/17.2"
p postgresql "GH releases (claim: none exist)" "https://api.github.com/repos/postgres/postgres/releases?per_page=1" $GH
p redis "git.redis.io" "https://git.redis.io/redis.git/info/refs?service=git-upload-pack"
p redis "download.redis.io" "https://download.redis.io/releases/"
p golang "go.googlesource.com" "https://go.googlesource.com/go/+refs?format=JSON"
p golang "storage.googleapis.com/golang 403" "https://storage.googleapis.com/golang/go1.25.0.linux-amd64.tar.gz" -r 0-100
p golang "dl.google.com" "https://dl.google.com/go/go1.25.0.linux-amd64.tar.gz" -r 0-100
p golang "go.dev/dl json" "https://go.dev/dl/?mode=json"
p falco "docs.falco.org DNS" "https://docs.falco.org/"
p falco "falco chart tgz 9.0.0 (re-published)" "https://github.com/falcosecurity/charts/releases/download/falco-9.0.0/falco-9.0.0.tgz" -r 0-100
p falco "falco download bucket" "https://download.falco.org/?prefix=packages/"
p falco "falco.org" "https://falco.org/docs/"
p vault "helm.releases.hashicorp.com" "https://helm.releases.hashicorp.com/index.yaml"
p vault "developer.hashicorp.com" "https://developer.hashicorp.com/vault/docs/release-notes"
p vault "www.vaultproject.io" "https://www.vaultproject.io/"
p vault "discuss.hashicorp.com HCSEC" "https://discuss.hashicorp.com/c/security/52"
p vault "checkpoint-api" "https://checkpoint-api.hashicorp.com/v1/check/vault"
p vault "endoflife.date" "https://endoflife.date/api/hashicorp-vault.json"
p vault "registry.terraform.io" "https://registry.terraform.io/.well-known/terraform.json"
p vault "go.hashi.co support policy" "https://go.hashi.co/vault-support-policy"
p vault "ghcr token" "https://ghcr.io/token?scope=repository:hashicorp/vault:pull"
p vault "releases.hashicorp.com index" "https://releases.hashicorp.com/vault/index.json"
p minio "dl.min.io legacy binary (410)" "https://dl.min.io/server/minio/release/linux-amd64/minio" -r 0-100
p minio "docs.min.io community" "https://docs.min.io/community/minio-object-store/"
p minio "docker.io minio/minio" "https://registry-1.docker.io/v2/minio/minio/manifests/latest" $OCI
p minio "quay.io minio/minio" "https://quay.io/v2/minio/minio/manifests/latest" $OCI
p minio "mirror.gcr.io minio" "https://mirror.gcr.io/v2/minio/minio/tags/list"
p traefik "mirror.gcr.io traefik" "https://mirror.gcr.io/v2/traefik/traefik/tags/list"
p traefik "docker hub library/traefik" "https://hub.docker.com/v2/repositories/library/traefik/tags/v3.5.0"
p linkerd "api.github.com rate limit" "https://api.github.com/rate_limit" $GH
p otel-collector "docker hub otel" "https://hub.docker.com/v2/repositories/otel/opentelemetry-collector-contrib/tags/0.127.0"
p flux "ghcr gotk-components oci" "https://ghcr.io/v2/fluxcd/flux-manifests/manifests/v2.9.6" $OCI
p flux "docker hub fluxcd" "https://hub.docker.com/v2/repositories/fluxcd/flux-cli/tags/"
p karpenter "public.ecr.aws token" "https://public.ecr.aws/token/"
