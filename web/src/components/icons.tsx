// Minimal stroke icon set (inline SVG, no dependency). 20px on a 24 viewBox.
import type { SVGProps } from "react";

function Svg(props: SVGProps<SVGSVGElement>) {
  return (
    <svg
      width="20" height="20" viewBox="0 0 24 24" fill="none"
      stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round"
      aria-hidden="true" {...props}
    />
  );
}

export const IconHome = (p: SVGProps<SVGSVGElement>) => (
  <Svg {...p}><path d="M3 10.5 12 3l9 7.5" /><path d="M5 9.5V21h14V9.5" /><path d="M9.5 21v-6h5v6" /></Svg>
);
export const IconInbox = (p: SVGProps<SVGSVGElement>) => (
  <Svg {...p}><path d="M3 13h4l2 3h6l2-3h4" /><path d="M4 13 6 5h12l2 8v6H4v-6Z" /></Svg>
);
export const IconWallet = (p: SVGProps<SVGSVGElement>) => (
  <Svg {...p}><path d="M3 7h15a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z" /><path d="M3 7l0-1a2 2 0 0 1 2-2h11" /><circle cx="16.5" cy="13" r="1.3" /></Svg>
);
export const IconActivity = (p: SVGProps<SVGSVGElement>) => (
  <Svg {...p}><path d="M3 12h4l2.5 6 5-14L17 12h4" /></Svg>
);
export const IconList = (p: SVGProps<SVGSVGElement>) => (
  <Svg {...p}><path d="M8 6h12M8 12h12M8 18h12" /><circle cx="4" cy="6" r="1" /><circle cx="4" cy="12" r="1" /><circle cx="4" cy="18" r="1" /></Svg>
);
export const IconShield = (p: SVGProps<SVGSVGElement>) => (
  <Svg {...p}><path d="M12 3 5 6v6c0 4 3 6.5 7 9 4-2.5 7-5 7-9V6l-7-3Z" /><path d="m9 12 2 2 4-4" /></Svg>
);
export const IconBook = (p: SVGProps<SVGSVGElement>) => (
  <Svg {...p}><path d="M5 4h11a2 2 0 0 1 2 2v14H7a2 2 0 0 1-2-2V4Z" /><path d="M5 18a2 2 0 0 1 2-2h11" /><path d="M9 8h6M9 11h5" /></Svg>
);
export const IconAdjust = (p: SVGProps<SVGSVGElement>) => (
  <Svg {...p}><path d="M4 8h11M4 8a2 2 0 1 0 4 0 2 2 0 0 0-4 0Z" /><path d="M20 16H9m11 0a2 2 0 1 1-4 0 2 2 0 0 1 4 0Z" /></Svg>
);
export const IconUsers = (p: SVGProps<SVGSVGElement>) => (
  <Svg {...p}><circle cx="9" cy="8" r="3" /><path d="M3 20a6 6 0 0 1 12 0" /><path d="M16 6a3 3 0 0 1 0 5.5" /><path d="M17 14.5a6 6 0 0 1 4 5.5" /></Svg>
);
export const IconPlus = (p: SVGProps<SVGSVGElement>) => (
  <Svg {...p}><path d="M12 5v14M5 12h14" /></Svg>
);
export const IconCheck = (p: SVGProps<SVGSVGElement>) => (
  <Svg {...p}><path d="m5 12 5 5 9-11" /></Svg>
);
export const IconX = (p: SVGProps<SVGSVGElement>) => (
  <Svg {...p}><path d="M6 6l12 12M18 6 6 18" /></Svg>
);
export const IconArrowRight = (p: SVGProps<SVGSVGElement>) => (
  <Svg {...p}><path d="M5 12h14M13 6l6 6-6 6" /></Svg>
);
export const IconInfo = (p: SVGProps<SVGSVGElement>) => (
  <Svg {...p} strokeWidth="1.6"><circle cx="12" cy="12" r="9" /><path d="M12 11v5" /><circle cx="12" cy="7.6" r="0.6" fill="currentColor" stroke="none" /></Svg>
);
export const IconLogout = (p: SVGProps<SVGSVGElement>) => (
  <Svg {...p}><path d="M15 4h4v16h-4" /><path d="M10 12h9M15 8l4 4-4 4" /></Svg>
);
export const IconZap = (p: SVGProps<SVGSVGElement>) => (
  <Svg {...p}><path d="M13 3 4 14h6l-1 7 9-11h-6l1-7Z" /></Svg>
);
export const IconChart = (p: SVGProps<SVGSVGElement>) => (
  <Svg {...p}><path d="M4 20V10M11 20V4M18 20v-7" /><path d="M2 20h20" /></Svg>
);
