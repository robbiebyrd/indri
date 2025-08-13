import {User} from "@/models/models"
import AsyncStorage from '@react-native-async-storage/async-storage';


export type Action = 'setUser'

export type UserDispatchMessage = {
    type: Action
    payload: User
    sessionId: string
}


const storeData = async (value: string) => {
    console.log('Storing data...', value);
    try {
        await AsyncStorage.setItem('sessionId', value);
    } catch (e) {
        return e
    }
};

export function dataHandler(state?: User, action?: UserDispatchMessage): User | undefined {
    switch (action?.type) {
        case 'setUser':
            if (action?.sessionId) {
                storeData(action.sessionId).finally()
            }
            return {...action.payload};
        default:
            return state
    }
}
