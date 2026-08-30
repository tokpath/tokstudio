import { create } from "zustand";

// Zustand 只放客户端交互提示，不存余额/账单等服务端事实。
type ToastState = {
  message: string;
  setMessage: (message: string) => void;
};

export const useToast = create<ToastState>((set) => ({
  message: "",
  setMessage: (message) => set({ message }),
}));
