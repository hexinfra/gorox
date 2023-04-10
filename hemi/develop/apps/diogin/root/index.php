<?php
ob_start('ob_gzhandler');
echo 'hello from php';
ob_end_flush();
