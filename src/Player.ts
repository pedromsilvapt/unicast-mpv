import NodeMpv from 'node-mpv';
import { Config } from './Config';
import { Future } from '@pedromsilva/data-future';
import { changeObjectCase } from './Utils/ChangeObjectCase';
import Case from 'case';

export function valueToMpv ( value : any ) {
    if ( typeof value === 'boolean' ) {
        return value ? 'yes' : 'no';
    } else {
        return value;
    }
}

export class Player {
    public config : Config;

    public mpv : NodeMpv;

    public status : PlayerStatus;
    
    protected observedProperties : string[] = [];

    constructor ( config : Config ) {
        this.config = config;

        const args : string[] = [ 
            '--player-operation-mode=pseudo-gui',
            '--force-window',
            '--terminal'
        ];

        const monitor = this.config.get( 'monitor', null );

        this.status = new PlayerStatus( this );

        if ( typeof monitor == 'number' ) {
            args.push( '--screen=' + monitor, '--fs-screen=' + monitor );
        }

        if ( this.config.get( 'onTop', false ) ) {
            args.push( '--ontop' );
        }

        if ( this.config.get( 'fullscreen', false ) ) {
            args.push( '--fs' );
        }

        if ( this.config.get( 'videoOutput', null ) != null ) {
            args.push( '--vo=' + this.config.get( 'videoOutput' ) );
        }

        if ( this.config.get( 'audioOutput', null ) != null ) {
            args.push( '--ao=' + this.config.get( 'audioOutput' ) );
        }

        if ( this.config.get( 'audioDevice', null ) != null ) {
            args.push( '--audio-device=' + this.config.get( 'audioDevice' ) );
        }

        if ( this.config.get( 'args', null ) != null ) {
            args.push( ...this.config.get( 'args' ) );
        }

        const subtitlesConfig = changeObjectCase( this.config.get( 'subtitles', {} ), 'kebab' );

        for ( let key in subtitlesConfig ) {
            const value = subtitlesConfig[ key ];

            if ( value === null || value === void 0 ) {
                continue;
            }

            args.push( '--sub-' + key + '=' + valueToMpv( value ) );
        }

        this.mpv = new NodeMpv( { binary: this.config.get( 'binary', null ), auto_restart: true }, args );
    }
    
    observeProperty ( property : string ) {
        this.observedProperties.push( property );

        if ( this.mpv.isRunning() ) {
            this.mpv.observeProperty( property );
        }
    }

    async start () {
        await this.mpv.start();

        for ( let property of this.observedProperties ) {
            this.mpv.observeProperty( property );
        }
    }

    setMultipleProperties ( properties : any ) {
        properties = changeObjectCase( properties, 'kebab' );

        this.mpv.setMultipleProperties( properties );
    }

    async load ( file : string, flags : LoadFlags = LoadFlags.Replace, options : LoadOptions ) {
        options = changeObjectCase( options, 'kebab' );

        // Force subtitles delay back to zero when playing a new media file
        if ( !( 'sub-delay' in options ) ) {
            options[ 'sub-delay' ] = 0;
        }

        const optionsString = Object.keys( options )
            .map( key => `${key}=${valueToMpv(options[ key ])}` );

        await this.mpv.load( file, flags, optionsString );

        this.mpv.adjustSubtitleTiming( 0 );
    }

    async stop () {
        const status = await this.status.get();

        if ( status.path ) {
            await this.mpv.stop();
        }

        return;
    }
}

export enum LoadFlags {
    Replace = 'replace',
    Append = 'append',
    AppendPlay = 'append-play'
}

export interface LoadOptions {
    start ?: number;
    pause ?: boolean;
    title ?: string;
    mediaTitle ?: string;
    
    // Subtitles
    subFixTiming ?: boolean;
    subFont ?: string;
    subColor ?: string;
    subBold ?: boolean;
    subItalic ?: boolean;
    
    subSpacing ?: 0;

    subBackColor ?: string;
    subBorderColor ?: string;
    subBorderSize ?: number;

    subShadowColor ?: string;
    subShadowOffset ?: number;

    subMarginX ?: number;
    subMarginY ?: number;

    lavfiComplex ?: string;

    [ key : string ] : any;
}

export interface StatusChange {
    property: string;
    value: any;
}

export interface StatusInfo {
    mute : boolean;
    pause : boolean;
    duration : number;
    position ?: number;
    volume : number;
    filename : string;
    path : string;
    mediaTitle : string;
    playlistPos : number;
    playlistCount : number;
    subScale : number;
    subVisibility : boolean;
    loop : string;
    fullscreen: boolean;
}

export const StatusInfoRequiredKeys = [
    'duration', 'position', 'filename', 'path', 
    'mediaTitle', 'playlistPos', 'playlistCount'
] as const;

export class PlayerStatus {
    public player : Player;

    protected lastStatus : StatusInfo = { 
        mute: false, 
        pause: false, 
        duration: 0, 
        position: 0, 
        volume: 100, 
        filename: null, 
        path: null, 
        mediaTitle: null,
        playlistPos: 0,
        playlistCount: 0,
        loop: "no",
        subVisibility: true,
        subScale: 1,
        fullscreen: false,
    };

    protected lastStatusFuture : Future<void> = null;

    constructor ( player : Player ) {
        this.player = player;
    }

    getSync () : StatusInfo {
        return this.lastStatus;
    }

    play () {
        for ( let key of StatusInfoRequiredKeys ) {
            delete this.lastStatus[key];
        }

        this.lastStatusFuture = new Future();
    }

    stop () {
        if ( this.lastStatus != null ) {
            this.lastStatus.path = null;
            this.lastStatus.filename = null;
        }

        if ( this.lastStatusFuture != null ) {
            const future = this.lastStatusFuture;

            this.lastStatusFuture = null;

            future.resolve();
        }
    }

    update ( status : StatusChange ) {
        this.lastStatus[ Case.camel( status.property ) ] = status.value;

        if ( this.lastStatusFuture != null ) {
            const missingKeys = StatusInfoRequiredKeys.filter(key => !(key in this.lastStatus));

            if (missingKeys.length == 0) {
                const future = this.lastStatusFuture;
    
                this.lastStatusFuture = null;
    
                future.resolve();
            }
        }
    }

    async get () : Promise<StatusInfo> {
        if ( this.lastStatusFuture !== null ) {
            await timeout( this.lastStatusFuture.promise, 5000 );
        }

        if ( this.lastStatus.path != null ) {
            if ( this.player.mpv.isRunning() ) {
                this.lastStatus.position = await this.player.mpv.getTimePosition();
            } else {
                this.stop();
            }
        }

        return this.lastStatus;
    }
}

export class TimeoutError extends Error {

}

export function timeout <T> ( promise : Promise<T>, duration : number, onTimeout : T | Promise<T> = null ) {
    return Promise.race( [
        promise,
        new Promise( ( resolve, reject ) => setTimeout( () => onTimeout ? resolve( onTimeout ) : reject( new TimeoutError() ), duration ) )
    ] );
}