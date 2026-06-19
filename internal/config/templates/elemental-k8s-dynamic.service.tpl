[Unit]
Description=Elemental K8s Dynamic Configuration
Before=k8s-config-installer.service
ConditionPathExists=!/run/elemental/k8s-dynamic-applied

[Service]
Type=oneshot
RemainAfterExit=yes
TimeoutSec={{ .Timeout }}
ExecStartPre=/bin/mkdir -p /run/elemental
ExecStart=/usr/bin/elemental3ctl k8s-dynamic apply --config {{ .ConfigPath }}
ExecStartPost=/bin/touch /run/elemental/k8s-dynamic-applied

[Install]
WantedBy=multi-user.target
