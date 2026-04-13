interface Slider {
  id: "temperature" | "exaggeration" | "cfgWeight"
  label: string
  leftLabel: string
  rightLabel: string
  min: number
  max: number
  step: number
  defaultValue: number
}

export const sliders: Slider[] = [
  {
    id: "temperature",
    label: "Creativity",
    leftLabel: "Consistent",
    rightLabel: "Expressive",
    min: 0,
    max: 2,
    step: 0.1,
    defaultValue: 0.8,
  },
  {
    id: "exaggeration",
    label: "Exaggeration",
    leftLabel: "Natural",
    rightLabel: "Dramatic",
    min: 0.25,
    max: 2,
    step: 0.05,
    defaultValue: 0.5,
  },
  {
    id: "cfgWeight",
    label: "CFG Weight",
    leftLabel: "Loose",
    rightLabel: "Guided",
    min: 0.2,
    max: 1,
    step: 0.05,
    defaultValue: 0.5,
  },
]
