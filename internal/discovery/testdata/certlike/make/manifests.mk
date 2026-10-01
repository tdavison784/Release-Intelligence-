helm-chart: $(bin_dir)/certmgr-$(VERSION).tgz

$(bin_dir)/certmgr-$(VERSION).tgz: $(bin_dir)/helm/certmgr/Chart.yaml
	$(HELM) package --app-version=$(VERSION) --version=$(VERSION) --destination "$(dir $@)" ./$(bin_dir)/helm/certmgr
