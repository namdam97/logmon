import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

// cn gộp class names có điều kiện và hợp nhất xung đột Tailwind.
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
