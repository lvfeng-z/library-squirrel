// Slot types for plugin system

export interface ViewSlot {
  slotId: string;
  name: string;
  component: any;
  order?: number;
  isPlugin?: boolean;
  isBuiltin?: boolean;
}

export interface EmbedSlot {
  slotId: string;
  name: string;
  position: string;
  component: any;
  order?: number;
  pluginId?: number;
}

export interface PanelSlot {
  slotId: string;
  name: string;
  position: string;
  component: any;
  order?: number;
  pluginId?: number;
}
