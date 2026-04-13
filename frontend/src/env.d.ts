/// <reference types="vite/client" />

declare module "*.vue" {
  import type { DefineComponent } from "vue";
  const component: DefineComponent<object, object, unknown>;
  export default component;
}

// Wails runtime types
declare namespace Wails {
  namespace Runtime {
    function EventsOn(eventName: string, callback: (...data: any[]) => void): void;
    function EventsOff(eventName: string): void;
    function Emit(eventName: string, ...data: any[]): void;
  }
}

// Extend window for Wails
interface Window {
  wails: {
    Events: {
      On(eventName: string, callback: (...data: any[]) => void): void;
      Off(eventName: string): void;
      Emit(eventName: string, ...data: any[]): boolean;
    };
  };
}
