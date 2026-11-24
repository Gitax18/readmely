"use client";

import { animate, MotionConfig, motion, useReducedMotion } from "motion/react";
import * as React from "react";

/** Easing shared by every animation, so movement feels like one system. */
export const EASE = [0.2, 0.7, 0.2, 1] as const;

/**
 * Wraps the app once. `reducedMotion="user"` turns movement into plain fades for visitors who ask
 * for less motion; the nonce lets Motion's own injected styles pass the content security policy.
 */
export function MotionProvider({
  children,
  nonce,
}: {
  children: React.ReactNode;
  nonce?: string | undefined;
}) {
  return (
    <MotionConfig
      reducedMotion="user"
      {...(nonce ? { nonce } : {})}
      transition={{ duration: 0.35, ease: EASE }}
    >
      {children}
    </MotionConfig>
  );
}

/**
 * Fades and lifts its content in as it scrolls into view. Content already on screen is left
 * alone, so nothing flickers on load; only what is below the fold waits, and it is hidden by a
 * class set after hydration, so the server markup carries no inline style (the content security
 * policy refuses it) and a visitor without JavaScript still reads everything.
 */
export function Reveal({
  children,
  delay = 0,
  className,
  as: Tag = "div",
}: {
  children: React.ReactNode;
  delay?: number;
  className?: string;
  as?: "div" | "section" | "li" | "article";
}) {
  const ref = React.useRef<HTMLElement>(null);
  React.useEffect(() => {
    const el = ref.current;
    if (!el || typeof IntersectionObserver === "undefined") return;
    if (el.getBoundingClientRect().top < window.innerHeight * 0.92) return;
    el.dataset.reveal = "hidden";
    if (delay) el.style.transitionDelay = `${delay}s`;
    const io = new IntersectionObserver(
      ([entry]) => {
        if (!entry?.isIntersecting) return;
        el.dataset.reveal = "shown";
        io.disconnect();
      },
      { rootMargin: "0px 0px -8% 0px" },
    );
    io.observe(el);
    return () => io.disconnect();
  }, [delay]);
  return (
    <Tag ref={ref as React.Ref<never>} className={className}>
      {children}
    </Tag>
  );
}

/**
 * Counts up to `value` when it first appears. The server renders the final number, so nothing is
 * wrong without JavaScript; with reduced motion the number just stays.
 */
export function AnimatedNumber({ value, className }: { value: number; className?: string }) {
  const ref = React.useRef<HTMLSpanElement>(null);
  const reduce = useReducedMotion();
  React.useEffect(() => {
    const el = ref.current;
    if (!el || reduce) return;
    const controls = animate(0, value, {
      duration: 0.9,
      ease: EASE,
      onUpdate: (n) => {
        el.textContent = Math.round(n).toLocaleString("en-US");
      },
    });
    return () => controls.stop();
  }, [reduce, value]);
  return (
    <span ref={ref} className={className}>
      {value.toLocaleString("en-US")}
    </span>
  );
}

export { motion };
