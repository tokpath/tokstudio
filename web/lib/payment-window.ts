// A new payment window has to be opened during the click that confirms the
// method. Opening it after the checkout request returns is blocked by browsers.

const paymentWindowName = "tokpath-gkash";

export function reservePaymentWindow(): Window | null {
  const opened = window.open("about:blank", paymentWindowName);
  if (!opened) return null;
  opened.document.open();
  opened.document.write("<!doctype html><title>Gkash</title><p>Opening payment page...</p>");
  opened.document.close();
  return opened;
}

export function submitPaymentForm(target: Window | null, url: string, fields: Record<string, string>) {
  if (!target || target.closed) return false;
  const doc = target.document;
  doc.open();
  doc.write("<!doctype html><title>Gkash</title><body></body>");
  doc.close();
  const form = doc.createElement("form");
  form.method = "POST";
  form.action = url;
  for (const [name, value] of Object.entries(fields)) {
    const input = doc.createElement("input");
    input.type = "hidden";
    input.name = name;
    input.value = value;
    form.append(input);
  }
  doc.body.append(form);
  form.submit();
  return true;
}

export function closePaymentWindow(target: Window | null) {
  if (target && !target.closed) target.close();
}
