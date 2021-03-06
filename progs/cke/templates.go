package cke

import "text/template"

var serviceTmpl = template.Must(template.New("cke.service").
	Parse(`[Unit]
Description=CKE container
Wants=network-online.target
After=network-online.target
ConditionPathExists={{ .ConfFile }}
ConditionPathExists={{ .CertFile }}
ConditionPathExists={{ .KeyFile }}

[Service]
Slice=machine.slice
Type=simple
KillMode=mixed
Restart=always
RestartSec=10s
ExecStart=/usr/bin/rkt run \
  --pull-policy never \
  --net=host \
  --dns=host \
  --hosts-entry=host \
  --hostname=%H \
  --volume certs,kind=host,source=/etc/ssl/certs,readOnly=true \
  --mount volume=certs,target=/etc/ssl/certs \
  --volume conf,kind=host,source=/etc/cke,readOnly=true \
  --mount volume=conf,target=/etc/cke \
  {{ .Image }} \
    --name cke \
    --readonly-rootfs=true \
  -- \
    --config={{ .ConfFile }}

[Install]
WantedBy=multi-user.target
`))
