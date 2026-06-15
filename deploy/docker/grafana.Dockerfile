FROM grafana/grafana-enterprise:13.0.2

ENV GF_AUTH_ANONYMOUS_ENABLED="true"
ENV GF_AUTH_ANONYMOUS_ORG_ROLE="Admin"
ENV GF_DEFAULT_APP_MODE="development"
ENV GF_LOG_FILTERS="plugin.filebeat-k8s-app:debug"
ENV GF_LOG_LEVEL="debug"
ENV GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS="filebeat-k8s-app"

COPY dist /var/lib/grafana/plugins/filebeat-k8s-app
COPY provisioning /etc/grafana/provisioning
