import { useEffect, useRef, type ReactNode } from "react";

// Drops an inset shadow onto the nearest scrollable ancestor (typically
// .ant-drawer-body) while there's more content below the fold — otherwise a
// form taller than the viewport gives no hint it's cut off. Reads the real
// scroll container via the DOM rather than rendering its own, so it doesn't
// have to fight Drawer's own flex/overflow layout.
const ScrollShadow = ({ children }: { children: ReactNode }) => {
  const anchorRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const scrollEl = anchorRef.current?.parentElement;
    if (!scrollEl) return;

    const check = () => {
      const hasMore = scrollEl.scrollHeight - scrollEl.scrollTop - scrollEl.clientHeight > 1;
      scrollEl.style.boxShadow = hasMore ? "inset 0 -16px 12px -14px rgba(0, 0, 0, 0.35)" : "";
    };

    check();
    scrollEl.addEventListener("scroll", check);
    const observer = new ResizeObserver(check);
    observer.observe(scrollEl);

    return () => {
      scrollEl.removeEventListener("scroll", check);
      observer.disconnect();
      scrollEl.style.boxShadow = "";
    };
  }, []);

  return (
    <>
      <div ref={anchorRef} style={{ display: "none" }} />
      {children}
    </>
  );
};

export default ScrollShadow;
