export const passwordPolicy = "密码须为 10–30 位，从数字、大写英文字母、小写英文字母、英文特殊符号中至少选用两类。";

export function validNewPassword(password: string): boolean {
  if (password.length < 10 || password.length > 30 || /[^!-~]/.test(password)) return false;
  return [/[0-9]/, /[A-Z]/, /[a-z]/, /[^0-9A-Za-z]/].filter(category => category.test(password)).length >= 2;
}
