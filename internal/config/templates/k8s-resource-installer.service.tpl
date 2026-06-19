[Unit]
Description=Kubernetes Resources Installer
After=k8s-config-installer.service
{{ if .Dynamic -}}
ConditionPathExists=/run/elemental/k8s-dynamic-deploy-resources
{{ else -}}
ConditionHost={{ .InitHostname }}
{{ end -}}

[Service]
Type=oneshot
TimeoutSec=900
Restart=on-failure
RestartSec=60
ExecStartPre=/bin/sh -c 'until [ "$(systemctl show -p SubState --value rke2-server.service)" = "running" ]; do sleep 10; done'
ExecStart=/bin/bash "{{ .ManifestDeployScript }}" 
ExecStartPost=/bin/sh -c "systemctl disable k8s-resource-installer.service"
ExecStartPost=/bin/sh -c "rm -rf /etc/systemd/system/k8s-resource-installer.service"
ExecStartPost=/bin/sh -c 'rm -rf "{{ .KubernetesDir }}"'

[Install]
WantedBy=multi-user.target
