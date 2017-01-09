<?php

$timestamp=time();
echo "time=" . $timestamp, PHP_EOL;
$region="sh";
$secretId="AKIDPSMPyxovTZIaZOlUWs2RWl6KALCv4AsT";
$secretKey="Kl1gxUnW27EQhKTT2GchNbJIHFiJ9qr0";
$url="vpc.api.qcloud.com/v2/index.php";
$Nonce=time();
echo "Nonce=" . $Nonce, PHP_EOL;
$vpcId="vpc-o5sjk6f1";
$subnetId="42";
$argsArr=array(
        "Action" => "ApplyIps",
        "Nonce" => $Nonce,
        "Region" => $region,
        "SecretId" => $secretId,
        "Timestamp" => $timestamp,
        "vpcId" => $vpcId,
        "subnetId" => $subnetId,
        "count" => 20,
);
ksort($argsArr);
$basicArgs=$url . "?" . http_build_query($argsArr);
#$basicArgs="vpc.api.qcloud.com/v2/index.php?Action=DescribeBmVpcEx&Nonce=1&Region=" . $region . "&SecretId=" . $secretId . "&Timestamp=" . $timestamp;
$srcStr="GET" . $basicArgs;
$signStr=urlencode(base64_encode(hash_hmac('sha1', $srcStr, $secretKey, true)));
#echo $signStr, PHP_EOL;
$ch = curl_init();
$request="https://" . $basicArgs . "&Signature=" . $signStr . "&vpcId=" .$vpcId;
echo $region . " " . $request, PHP_EOL;
curl_setopt($ch, CURLOPT_URL, $request);
$result=curl_exec($ch);
curl_close($ch);
echo PHP_EOL;
echo PHP_EOL;
?>