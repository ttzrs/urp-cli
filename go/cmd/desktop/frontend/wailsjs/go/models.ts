export namespace main {
	
	export class CtxInfo {
	    tokens: number;
	    files: number;
	
	    static createFrom(source: any = {}) {
	        return new CtxInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tokens = source["tokens"];
	        this.files = source["files"];
	    }
	}
	export class FocusState {
	    target: string;
	    depth: number;
	
	    static createFrom(source: any = {}) {
	        return new FocusState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.target = source["target"];
	        this.depth = source["depth"];
	    }
	}
	export class Session {
	    id: string;
	    title: string;
	    updatedAt: number;
	    messages: number;
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.updatedAt = source["updatedAt"];
	        this.messages = source["messages"];
	    }
	}
	export class Status {
	    graphConnected: boolean;
	    project: string;
	    eventCount: number;
	    workers: number;
	    focus?: FocusState;
	    ctx: CtxInfo;
	    memgraphUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.graphConnected = source["graphConnected"];
	        this.project = source["project"];
	        this.eventCount = source["eventCount"];
	        this.workers = source["workers"];
	        this.focus = this.convertValues(source["focus"], FocusState);
	        this.ctx = this.convertValues(source["ctx"], CtxInfo);
	        this.memgraphUrl = source["memgraphUrl"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

