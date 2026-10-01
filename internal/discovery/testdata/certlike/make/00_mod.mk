repo_name := github.com/example/certmgr
GOLDFLAGS := -w -s \
	-X github.com/example/certmgr/pkg/util.AppVersion=$(VERSION)
build_names := controller webhook
helm_chart_source_dir := deploy/charts/certmgr
