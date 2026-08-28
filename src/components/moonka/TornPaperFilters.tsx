export function TornPaperFilters() {
  return (
    <svg width="0" height="0" style={{ position: "absolute" }} aria-hidden="true">
      <filter id="torn">
        <feTurbulence type="fractalNoise" baseFrequency="0.015 0.07" numOctaves={4} seed={9} result="n" />
        <feDisplacementMap in="SourceGraphic" in2="n" scale={11} xChannelSelector="R" yChannelSelector="G" />
      </filter>
      <filter id="torn2">
        <feTurbulence type="fractalNoise" baseFrequency="0.02 0.09" numOctaves={3} seed={24} result="n" />
        <feDisplacementMap in="SourceGraphic" in2="n" scale={8} xChannelSelector="R" yChannelSelector="G" />
      </filter>
    </svg>
  );
}
