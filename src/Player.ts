import Mpv from 'node-mpv';
import { Config } from './Config';
import { Future } from '@pedromsilva/data-future';

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

    async stop () {
        const status = await this.status.get();

        if ( status.path ) {
            return this.mpv.stop();
        }

        return;
    }
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