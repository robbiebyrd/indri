import {createContext} from 'react'
import {User} from "@/models/models"

interface UserStateContextType {
    userState?: User;
    dispatch: any;
}

export const UserStateContext = createContext<UserStateContextType | undefined>(undefined);

export const initialState: User | undefined = undefined
