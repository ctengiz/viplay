export namespace main {
	
	export class MediaItem {
	    name: string;
	    path: string;
	    url: string;
	    kind: string;
	
	    static createFrom(source: any = {}) {
	        return new MediaItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.url = source["url"];
	        this.kind = source["kind"];
	    }
	}

}

