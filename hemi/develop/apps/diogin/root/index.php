<?php
ob_start('ob_gzhandler');
echo "hello from php\n";
ob_end_flush();
