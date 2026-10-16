package portal

func responsibleRefundOperations(base string) []operation {
	fx := executionValuationFields()
	for i := range fx {
		fx[i].Required = false
	}
	merchant := f("merchant_id", "当前责任经营方独立配置的付款商户", "text")
	proof := []field{merchant, f("external.external_reference", "实际付款唯一流水", "text"), f("external.money.currency", "实际原币种或准确资产代码", "text"), f("external.money.amount", "本次实际原币金额", "decimal"), f("external.executed_at", "真实执行时刻", "datetime"), f("external.evidence_reference", "实际已执行凭据", "textarea"), f("verification_evidence", "本次接收目的地与执行事实核实依据", "textarea")}
	return []operation{
		{Name: "verify_responsible_evm", Label: "责任方自身钱包：只读核实实际链上退款", Method: "POST", Path: base + "/{id}/verify_responsible_evm", Fields: append([]field{merchant, f("evm.transaction_hash", "本责任方实际退款交易哈希", "text"), f("evm.log_index", "真实Transfer日志索引（0x）", "text"), f("verification_evidence", "本次链上核实依据", "textarea")}, fx...), Confirmation: "只读查询当前责任经营方钱包的真实交易；不签名、不发送、不读取原经营方商户。"},
		{Name: "verify_responsible_external", Label: "责任方自身NOW：登记外部实际退款", Method: "POST", Path: base + "/{id}/verify_responsible_external", Fields: append(append([]field{}, proof...), fx...), Confirmation: "已核实当前责任经营方的实际外部付款，系统不假称自动退款。"},
		{Name: "verify_responsible_bank", Label: "责任方自身银行：登记已核实实际退款", Method: "POST", Path: base + "/{id}/verify_responsible_bank", Fields: append(append([]field{}, proof...), fx...), Confirmation: "已核实本责任经营方银行的实际付款，不使用原经营方资金，不把付款意图记为成功。"},
	}
}
