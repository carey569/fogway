package provider

// zhipuResetList is customer-package-reset/list?targetType=PERSONAL as the
// reporter of #1191 captured it on bigmodel.cn, the account and record ids
// and times redacted there and filled in here with the reply's own shape:
// numeric ids, times as "YYYY-MM-DD HH:mm:ss".
const zhipuResetList = `{
  "code": 200,
  "msg": "操作成功",
  "data": {
    "customerId": 0,
    "targetType": "PERSONAL",
    "organizationId": null,
    "projectId": null,
    "lastFiveHourResetTime": "2026-10-06 21:14:03",
    "lastWeekResetTime": null,
    "fiveHourResets": [
      {
        "recordId": 0,
        "grantType": "DIRECT",
        "expireTime": "2026-11-02 23:59:59",
        "available": true
      }
    ],
    "weekResets": [
      {
        "recordId": 0,
        "grantType": "DIRECT",
        "expireTime": "2026-10-30 23:59:59",
        "available": true
      },
      {
        "recordId": 0,
        "grantType": "DIRECT",
        "expireTime": "2026-10-04 23:59:59",
        "available": false
      }
    ]
  },
  "success": true
}`
