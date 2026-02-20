# Test Kopo Kopo Webhook
# This script simulates a successful payment webhook from Kopo Kopo

$webhookUrl = "YOUR_RAILWAY_URL/api/webhooks/payment"
$orderID = "YOUR_ORDER_ID"

$payload = @{
    event_type = "buygoods_transaction_received"
    resource = @{
        id = "test-txn-123"
        status = "Success"
        reference = "ABC123DEF456"
        amount = "20"
        metadata = @{
            order_id = $orderID
        }
    }
} | ConvertTo-Json -Depth 5

Write-Host "Payload to send:"
Write-Host $payload
Write-Host ""
Write-Host "Note: You need to add the X-KopoKopo-Signature header with proper HMAC signature"
Write-Host "URL: $webhookUrl"
