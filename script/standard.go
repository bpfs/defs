package script

// isPubKeyHashScript 返回传递的脚本是否是标准的 支付到公钥哈希脚本。
func isPubKeyHashScript(script []byte) bool {
	return extractPubKeyHash(script) != nil
}

// extractPubKeyHash 从传递的脚本中提取公钥哈希值，如果它 是标准的支付到公钥哈希脚本。 否则将返回 nil。
func extractPubKeyHash(script []byte) []byte {
	// A pay-to-pubkey-hash script is of the form:
	//  OP_DUP OP_HASH160 <20-byte hash> OP_EQUALVERIFY OP_CHECKSIG

	// logger.Printf("\nscript:\t\t%d\n", len(script))
	// logger.Printf("script[3:23]:\t%x\n", script[3:23])
	// logger.Printf("script[24]:\t%d\n\n", script[24])
	if len(script) == 25 &&
		script[0] == OP_DUP &&
		script[1] == OP_HASH160 &&
		script[2] == OP_DATA_20 &&
		script[23] == OP_EQUALVERIFY &&
		script[24] == OP_CHECKSIG {

		return script[3:23]
	}

	return nil
}
