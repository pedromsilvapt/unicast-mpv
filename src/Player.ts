import Mpv from 'node-mpv';
import { Config } from './Config';
import { Future } from '@pedromsilva/data-future';
import { changeObjectCase } from './Utils/ChangeObjectCase';

export function valueToMpv ( value : any ) {
    if ( typeof value === 'boolean' ) {
        return value ? 'yes' : 'no';
    } else {
        return value;
    }
}

export class Player {
    public config : Config;

    public mpv : any;

    public status : PlayerStatus;
    
    protected observedPropertiesId : number = 13;

    protected observedProperties : string[] = [];

    constructor ( config : Config ) {
        this.config = config;

        const args : string[] = [ 
            '--player-operation-mode=pseudo-gui',
            '--force-window',
            '--idle=' + ( this.config.get( 'quitOnStop' ) ? 'once' : 'yes' ),
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

        const subtitlesConfig = changeObjectCase( this.config.get( 'subtitles', {} ), 'kebab' );

        for ( let key in subtitlesConfig ) {
            const value = subtitlesConfig[ key ];

            if ( value === null || value === void 0 ) {
                continue;
            }

            args.push( '--sub-' + key + '=' + valueToMpv( value ) );
        }

        this.mpv = new Mpv( { binary: this.config.get( 'binary', null ), auto_restart: true }, args );
    }
    
    observeProperty ( property : string ) {
        this.observedProperties.push( property );

        if ( this.mpv.isRunning() ) {
            this.mpv.observeProperty( property, 12 + this.observedProperties.length );
        }
    }

    async start () {
        await this.mpv.start();

        for ( let [ index, property ] of this.observedProperties.entries() ) {
            this.mpv.observeProperty( property, 13 + index );
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
}

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
        subScale: 1
    };

    protected lastStatusFuture : Future<void> = null;

    constructor ( player : Player ) {
        this.player = player;
    }

    getSync () : StatusInfo {
        return this.lastStatus;
    }

    play () {
        this.lastStatus = null;

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

    update ( status : StatusInfo ) {
        if ( status != null && this.lastStatus != null ) {
            status.position = this.lastStatus.position;
        }

        this.lastStatus = status;

        if ( this.lastStatusFuture != null ) {
            const future = this.lastStatusFuture;

            this.lastStatusFuture = null;

            future.resolve();
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

export function timeout <T> ( promise : Promise<T>, duration : number, onTimeout : T | Promise<T> = Promise.reject( new TimeoutError() ) ) {
    return Promise.race( [
        promise,
        new Promise( resolve => setTimeout( () => resolve( onTimeout ), duration ) )
    ] );
}