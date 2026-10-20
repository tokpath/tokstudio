// The named window must be opened during the click that confirms KVoucher.
// The form is submitted from this page into that window after checkout returns,
// so the current page stays in place.

const paymentWindowName = "tokpath-gkash";

export function reservePaymentWindow(): Window | null {
  return window.open("about:blank", paymentWindowName);
}

export function submitPaymentForm(target: Window | null, url: string, fields: Record<string, string>) {
  if (!target || target.closed) return false;
  const form = document.createElement("form");
  form.method = "POST";
  form.action = url;
  form.target = paymentWindowName;
  form.style.display = "none";
  for (const [name, value] of Object.entries(fields)) {
    const input = document.createElement("input");
    input.type = "hidden";
    input.name = name;
    input.value = value;
    form.append(input);
  }
  document.body.append(form);
  form.submit();
  form.remove();
  return true;
}

export function closePaymentWindow(target: Window | null) {
  if (target && !target.closed) target.close();
}
