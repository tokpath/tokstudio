package portal

func responsibleRefundOperations(base string) []operation {
	fx := executionValuationFields("external.money.currency")
	merchant := f("merchant_id", "当前责任经营方独立配置的付款商户", "text")
	proof := []field{merchant, f("external.external_reference", "实际付款唯一流水", "text"), f("external.money.currency", "付款币种", "text"), f("external.money.amount", "本次实际原币金额", "decimal"), f("external.executed_at", "真实执行时刻", "datetime"), f("external.evidence_reference", "实际已执行凭据", "textarea"), f("verification_evidence", "本次接收目的地与执行事实核实依据", "textarea")}
	return []operation{
		{Name: "verify_responsible_bank", Label: "责任方自身银行：登记已核实实际退款", Method: "POST", Path: base + "/{id}/verify_responsible_bank", Fields: append(append([]field{}, proof...), fx...), Confirmation: "已核实本责任经营方银行的实际付款，不使用原经营方资金，不把付款意图记为成功。"},
	}
}
