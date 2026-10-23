"use client";

import { useEffect, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ErrorPanel } from "./shared";

export type RecordChoice = { value: string; label: string; defaults?: Record<string, unknown> };

// One field searches and chooses. Typing asks for matches; choosing a row keeps its id.
export function RecordPicker({ label, required, invalid, disabled, ready = true, value, displayLabel, options, placeholder, emptyLabel, wildcardLabel, loading, error, onRetry, moreLabel, onMore, onQuery, onChoose }: {
  label: string;
  required?: boolean;
  invalid?: boolean;
  disabled?: boolean;
  ready?: boolean;
  value: string;
  displayLabel: string;
  options: RecordChoice[];
  placeholder: string;
  emptyLabel: string;
  wildcardLabel?: string;
  loading?: boolean;
  error?: unknown;
  onRetry?: () => void;
  moreLabel?: string;
  onMore?: () => void;
  onQuery: (query: string) => void;
  onChoose: (choice: RecordChoice | null) => void;
}) {
  const listId = useId();
  const root = useRef<HTMLDivElement>(null);
  const list = useRef<HTMLUListElement>(null);
  const chooseRef = useRef(onChoose);
  chooseRef.current = onChoose;
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<string | null>(null);
  const [box, setBox] = useState<{ top: number; left: number; width: number }>();
  const picked = useRef("");
  const waiting = !!loading && options.length === 0 && !value;
  const inactive = !!disabled || !ready || waiting;
  const shown = draft ?? displayLabel;
  const rows: RecordChoice[] = [
    ...(wildcardLabel ? [{ value: "*", label: wildcardLabel }] : []),
    ...(value && value !== "*" && !options.some(item => item.value === value) && displayLabel ? [{ value, label: displayLabel }] : []),
    ...options.filter(item => item.value !== "*"),
  ];

  useEffect(() => {
    if (!draft) { picked.current = ""; return; }
    if (picked.current === draft) return;
    const match = rows.find(item => item.value === draft || item.label === draft);
    if (!match || match.value === value) return;
    picked.current = draft;
    chooseRef.current(match);
    setDraft(null);
    setOpen(false);
  }, [draft, rows, value]);

  useEffect(() => {
    if (!open) return;
    const place = () => {
      const rect = root.current?.getBoundingClientRect();
      if (rect) setBox({ top: rect.bottom + 4, left: rect.left, width: rect.width });
    };
    const close = (event: PointerEvent) => {
      const target = event.target as Node;
      if (root.current?.contains(target) || list.current?.contains(target)) return;
      setOpen(false);
      setDraft(null);
    };
    place();
    window.addEventListener("scroll", place, true);
    window.addEventListener("resize", place);
    document.addEventListener("pointerdown", close);
    return () => {
      window.removeEventListener("scroll", place, true);
      window.removeEventListener("resize", place);
      document.removeEventListener("pointerdown", close);
    };
  }, [open]);

  function reveal() {
    if (inactive) return;
    setDraft("");
    onQuery("");
    setOpen(true);
  }
  function type(next: string) {
    setDraft(next);
    onQuery(next);
    setOpen(true);
    const match = rows.find(item => item.value === next || item.label === next);
    if (match) { onChoose(match); setDraft(null); setOpen(false); }
  }
  function choose(choice: RecordChoice | null) {
    onChoose(choice);
    setDraft(null);
    setOpen(false);
  }

  const menu = open && box ? <ul ref={list} id={listId} role="listbox" aria-label={label} data-record-picker-menu="" style={{ top: box.top, left: box.left, width: box.width, pointerEvents: "auto" }} className="pointer-events-auto fixed z-[80] max-h-60 overflow-auto rounded-control border border-hairline bg-canvas-raised py-1 text-sm shadow-lg">
    <li><button type="button" role="option" aria-selected={value === ""} className="block min-h-11 w-full px-3 text-left text-ink-secondary hover:bg-canvas" onMouseDown={event => event.preventDefault()} onClick={() => choose(null)}>{emptyLabel}</button></li>
    {rows.map(item => <li key={item.value}><button type="button" role="option" aria-selected={item.value === value} className="block min-h-11 w-full px-3 text-left hover:bg-canvas" onMouseDown={event => event.preventDefault()} onClick={() => choose(item)}>{item.label}</button></li>)}
    {loading && <li className="px-3 py-2 text-xs text-ink-mute">{placeholder}</li>}
    {onMore && moreLabel && <li className="px-2 py-1"><Button type="button" size="sm" variant="outline" disabled={disabled} onMouseDown={event => event.preventDefault()} onClick={onMore}>{moreLabel}</Button></li>}
  </ul> : null;

  return <div ref={root} className="relative mt-2 min-w-0">
    <div className={`flex min-h-11 items-center rounded-control border ${invalid ? "border-danger" : "border-hairline"} bg-canvas-raised ${inactive ? "opacity-60" : ""}`}>
      <input role="combobox" aria-label={label} aria-expanded={open} aria-controls={listId} aria-autocomplete="list" aria-invalid={invalid || undefined} className="min-h-11 min-w-0 flex-1 bg-transparent px-3 py-2 text-sm text-ink outline-none placeholder:text-ink-mute" value={shown} required={required} disabled={inactive} placeholder={placeholder} onFocus={reveal} onChange={event => type(event.target.value)} onKeyDown={event => { if (event.key === "Enter") event.preventDefault(); if (event.key === "Escape") { setOpen(false); setDraft(null); } }} />
      <button type="button" tabIndex={-1} aria-hidden className="grid h-11 w-11 place-items-center text-ink-mute" disabled={inactive} onMouseDown={event => event.preventDefault()} onClick={() => open ? (setOpen(false), setDraft(null)) : reveal()}><ChevronDown size={16} /></button>
    </div>
    {menu && createPortal(menu, document.body)}
    {!!error && onRetry && <div className="mt-2"><ErrorPanel compact error={error} retry={onRetry} /></div>}
  </div>;
}
