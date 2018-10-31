etcd schema
===========

## `<prefix>/leader/`

This prefix is used to elect a leader `neco-updater` who is responsible to invoke
update process to `neco-worker`.

## `<prefix>/bootservers/<LRN>`

This prefix is current available boot servers. Current available boot server is
registered as a key `LRN`.

The value is a serialized form of `artifacts.go`.

## `<prefix>/current`

A leader of `neco-updater` creates and updates this key.

The value is a JSON object with these fields:

Name      | Type   | Description
----      | ----   | -----------
`version` | string | Target `neco` version to be updated for all `servers`.
`servers` | []int  | LRNs of current available boot servers under update. This is created using `<prefix>/bootservers`.
`stop`    | bool   | If `true`, `neco-worker` stops the update process.

```json
{
    "version": "1.2.3-1",
    "servers": [1, 2, 3],
    "stop": false
}
```

`neco-worker` watches this key to start a new update process.
If `stop` becomes true, `neco-worker` should stop the ongoing update process immediately.

## `<prefix>/status/<LRN>`

`neco-worker` creates and updates this key.

The value is a JSON object with these fields:

Name       | Type   | Description
----       | ----   | -----------
`version`  | string | Target `neco` version to be updated.
`finished` | bool   | Update to `version` successfully completed.
`error`    | bool   | If `true`, the update process was aborted due to an error.
`message`  | string | Description of an error.

```json
{
    "version": "1.2.3-1",
    "finished": false,
    "error": true,
    "message": "cke update failed"
}
```

`neco-updater` watches these keys to wait all workers to complete update process,
or detect errors during updates.

## `<prefix>/notification`

This prefix is configuration of the notification service.

```json
{
    "webhook": "https://<webhook url>"
}
```

Name      | Type   | Description
----      | ----   | -----------
`webhook` | string | Slack web hook URL.

## `<prefix>/vault-unseal-key`

Vault unseal key for unsealing automatically.

## `<prefix>/vault-root-token`

Vault root token for automatic setup for [dctest](../dctest/).
This key does not exist by default.